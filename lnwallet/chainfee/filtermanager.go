package chainfee

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/rpcclient"
)

const (
	// fetchFilterInterval is the interval between successive fetches of
	// our peers' feefilters.
	fetchFilterInterval = time.Minute * 5

	// medianWindow is the number of historical values that the
	// filterManager will track.
	//
	// Assuming that each polling interval polls 8 outbound peers, the
	// value of 48 gives us 6 polling intervals which is ~3 blocks worth of
	// data.
	medianWindow = 48

	// minNumFilters is the minimum number of feefilters we need during a
	// polling interval. If we have fewer than this, we won't consider the
	// data.
	minNumFilters = 6
)

var (
	// errNoFilters is an error that's returned if fetchMedianFilter is
	// called and there is no data available.
	errNoFilters = fmt.Errorf("no data available")
)

// filterManager is responsible for determining an acceptable minimum fee to
// use based on our peers' feefilter values.
type filterManager struct {
	// values stores the historical feefilter values we'll use to come up
	// with an acceptable minimum feerate.
	values    []SatPerKWeight
	valuesMtx sync.RWMutex

	fetchFunc func() ([]SatPerKWeight, error)

	wg   sync.WaitGroup
	quit chan struct{}
}

// newFilterManager constructs a filterManager. If bitcoind is true, we'll use
// the dedicated fetchBitcoindFilters function. Else, we'll use
// fetchBtcdFilters.
func newFilterManager(cb func() ([]SatPerKWeight, error)) *filterManager {
	f := &filterManager{
		values: make([]SatPerKWeight, 0, medianWindow),
		quit:   make(chan struct{}),
	}

	f.fetchFunc = cb

	return f
}

// Start starts the filterManager.
func (f *filterManager) Start() {
	f.wg.Add(1)
	go f.fetchPeerFilters()
}

// Stop stops the filterManager.
func (f *filterManager) Stop() {
	close(f.quit)
	f.wg.Wait()
}

// fetchPeerFilters fetches our peers' feefilter values and adds them to our
// historical values.
func (f *filterManager) fetchPeerFilters() {
	filterTicker := time.NewTicker(fetchFilterInterval)
	defer filterTicker.Stop()

	for {
		select {
		case <-filterTicker.C:
			filters, err := f.fetchFunc()
			if err != nil {
				log.Errorf("Encountered err while fetching "+
					"fee filters: %v", err)
				return
			}

			f.updateWindow(filters)

		case <-f.quit:
			return
		}
	}
}

// fetchMedianFilter fetches the moving median of our historical feefilters.
func (f *filterManager) fetchMedianFilter() (SatPerKWeight, error) {
	f.valuesMtx.RLock()
	defer f.valuesMtx.RUnlock()

	if len(f.values) == 0 {
		// Return errNoFilters so the caller knows to ignore this
		// output and continue.
		return 0, errNoFilters
	}

	// Fetch the moving median.
	median := med(f.values)
	return median, nil
}

// chooseMinFee takes in the minimum relay fee and compares it against our
// median filter, returning whichever is highest. This function does not need
// to acquire the lock.
func (f *filterManager) chooseMinFee(minRelayFee SatPerKWeight) SatPerKWeight {
	filterFee, err := f.fetchMedianFilter()
	if err == errNoFilters {
		return minRelayFee
	}

	if filterFee > minRelayFee {
		return filterFee
	}

	return minRelayFee
}

type bitcoindPeerInfoResp struct {
	Inbound      bool    `json:"inbound"`
	MinFeeFilter float64 `json:"minfeefilter"`
}

func fetchBitcoindFilters(client *rpcclient.Client) ([]SatPerKWeight, error) {
	resp, err := client.RawRequest("getpeerinfo", nil)
	if err != nil {
		return nil, err
	}

	var peerResps []bitcoindPeerInfoResp
	err = json.Unmarshal(resp, &peerResps)
	if err != nil {
		return nil, err
	}

	// We filter for outbound peers since it is harder for an attacker to
	// be our outbound peer and therefore harder for them to manipulate us
	// into broadcasting high-fee or low-fee transactions.
	var outboundPeerFilters []SatPerKWeight
	for _, peerResp := range peerResps {
		if peerResp.Inbound {
			continue
		}

		// The value that bitcoind returns for the "minfeefilter" field
		// is in fractions of a bitcoin that represents the satoshis
		// per KvB. We need to convert this fraction to whole satoshis
		// by multiplying with COIN. Then we need to convert the
		// sats/KvB to sats/KW.
		//
		// Convert the sats/KvB from fractions of a bitcoin to whole
		// satoshis.
		filterKVByte := SatPerKVByte(
			peerResp.MinFeeFilter * btcutil.SatoshiPerBitcoin,
		)

		if !checkFilterBounds(filterKVByte) {
			continue
		}

		// Convert KvB to KW and add it to outboundPeerFilters.
		outboundPeerFilters = append(
			outboundPeerFilters, filterKVByte.FeePerKWeight(),
		)
	}

	// Check that we have enough data to use. We don't return an error as
	// that would stop the querying goroutine.
	if len(outboundPeerFilters) < minNumFilters {
		return nil, nil
	}

	return outboundPeerFilters, nil
}

func fetchBtcdFilters(client *rpcclient.Client) ([]SatPerKWeight, error) {
	resp, err := client.GetPeerInfo()
	if err != nil {
		return nil, err
	}

	var outboundPeerFilters []SatPerKWeight
	for _, peerResp := range resp {
		// We filter for outbound peers since it is harder for an
		// attacker to be our outbound peer and therefore harder for
		// them to manipulate us into broadcasting high-fee or low-fee
		// transactions.
		if peerResp.Inbound {
			continue
		}

		// The feefilter is already in units of sat/KvB.
		filter := SatPerKVByte(peerResp.FeeFilter)

		if !checkFilterBounds(filter) {
			continue
		}

		outboundPeerFilters = append(
			outboundPeerFilters, filter.FeePerKWeight(),
		)
	}

	// Check that we have enough data to use. We don't return an error as
	// that would stop the querying goroutine.
	if len(outboundPeerFilters) < minNumFilters {
		return nil, nil
	}

	return outboundPeerFilters, nil
}

// updateWindow takes a slice of feefilter values and adds them to the moving
// window.
func (f *filterManager) updateWindow(feeFilters []SatPerKWeight) {
	// If there are no elements, don't update.
	numElements := len(feeFilters)
	if numElements == 0 {
		return
	}

	f.valuesMtx.Lock()
	defer f.valuesMtx.Unlock()

	if len(f.values)+numElements <= medianWindow {
		// If there's room, we simply append.
		f.values = append(f.values, feeFilters...)
	} else {
		// Otherwise, we'll roll off the excess elements.
		numExcess := len(f.values) + numElements - medianWindow
		f.values = f.values[numExcess:]
		f.values = append(f.values, feeFilters...)
	}

	// Log the new moving median.
	loggingMedian := med(f.values)
	log.Debugf("filterManager updated moving median to: %v",
		loggingMedian.FeePerKVByte())
}

// checkFilterBounds returns false if the filter is unusable and true if it is.
func checkFilterBounds(filter SatPerKVByte) bool {
	// Ignore values of 0 and MaxSatoshi. A value of 0 likely means that
	// the peer hasn't sent over a feefilter and a value of MaxSatoshi
	// means the peer is using bitcoind and is in IBD.
	switch filter {
	case 0:
		return false

	case btcutil.MaxSatoshi:
		return false
	}

	// TODO: ignore values outside of a certain band (configurable?)
	return true
}

// med calculates the median of a slice.
// NOTE: Passing in an empty slice will panic!
func med(f []SatPerKWeight) SatPerKWeight {
	// Copy the original slice so that sorting doesn't modify the original.
	fCopy := make([]SatPerKWeight, len(f))
	copy(fCopy, f)

	sort.Slice(fCopy, func(i, j int) bool {
		return fCopy[i] < fCopy[j]
	})

	var median SatPerKWeight

	numElements := len(fCopy)
	switch numElements % 2 {
	case 0:
		// There's an even number of elements, so we need to average.
		middle := numElements / 2
		upper := fCopy[middle]
		lower := fCopy[middle-1]
		median = (upper + lower) / 2

	case 1:
		median = fCopy[numElements/2]
	}

	return median
}
