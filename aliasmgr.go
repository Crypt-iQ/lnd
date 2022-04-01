package lnd

import (
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/lightningnetwork/lnd/htlcswitch/hop"
	"github.com/lightningnetwork/lnd/kvdb"
	"github.com/lightningnetwork/lnd/lnwire"
)

var (
	// aliasBucket stores aliases as keys and their base SCIDs as values.
	// This is used to populate the maps that the aliasMgr uses. The keys
	// are alias SCIDs and the values are their respective base SCIDs. This
	// is used instead of the other way around (base -> alias...) because
	// updating an alias would require fetching all the existing aliases,
	// adding another one, and then flushing the write to disk. This is
	// inefficient compared to N 1:1 mappings at the cost of marginally
	// more disk space.
	aliasBucket = []byte("alias-bucket")

	// confirmedBucket stores whether or not a given base SCID should no
	// longer have entries in the ToBase maps. The key is the SCID that is
	// confirmed with 6 confirmations and is public, and the value is
	// empty.
	confirmedBucket = []byte("base-bucket")

	// aliasAllocBucket is a root-level bucket that stores the last alias
	// that was allocated. It is used to allocate a new alias when
	// requested.
	aliasAllocBucket = []byte("alias-alloc-bucket")

	// lastAliasKey is a key in the aliasAllocBucket whose value is the
	// last allocated alias ShortChannelID. This will be updated upon calls
	// to requestAlias.
	lastAliasKey = []byte("last-alias-key")

	// invoiceAliasBucket is a root-level bucket that stores the alias
	// SCIDs that our peers send us in the funding_locked TLV. The keys are
	// the ChannelID generated from the FundingOutpoint and the values are
	// the remote peer's alias SCID.
	invoiceAliasBucket = []byte("invoice-alias-bucket")

	// byteOrder denotes the byte order of database (de)-serialization
	// operations.
	byteOrder = binary.BigEndian

	// startingAlias is the first alias ShortChannelID that will get
	// assigned by requestAlias. The starting BlockHeight is chosen so that
	// legitimate SCIDs in integration tests aren't mistaken for an alias.
	startingAlias = lnwire.ShortChannelID{
		BlockHeight: 10000,
		TxIndex:     0,
		TxPosition:  0,
	}

	// errNoBase is returned when a base SCID isn't found.
	errNoBase = fmt.Errorf("no base found")

	// errNoPeerAlias is returned when the peer's alias for a given
	// channel is not found.
	errNoPeerAlias = fmt.Errorf("no peer alias found")
)

// aliasMgr is a struct that handles aliases for LND. It has an underlying
// database that can allocate aliases for channels, stores the peer's last
// alias for use in our hop hints, and contains mappings that both the Switch
// and Gossiper use.
type aliasMgr struct {
	backend kvdb.Backend

	// baseToSet is a mapping from the "base" SCID to the set of aliases
	// for this channel. This mapping includes all channels that
	// negotiated the option-scid-alias feature bit.
	baseToSet map[lnwire.ShortChannelID][]lnwire.ShortChannelID

	// aliasToBase is a mapping that maps all aliases for a given channel
	// to its base SCID. This is only used for channels that have
	// negotiated option-scid-alias feature bit.
	aliasToBase map[lnwire.ShortChannelID]lnwire.ShortChannelID

	sync.RWMutex
}

func newAliasMgr(db kvdb.Backend) (*aliasMgr, error) {
	a := &aliasMgr{backend: db}
	a.baseToSet = make(
		map[lnwire.ShortChannelID][]lnwire.ShortChannelID,
	)
	a.aliasToBase = make(
		map[lnwire.ShortChannelID]lnwire.ShortChannelID,
	)

	err := a.populateMaps()
	return a, err
}

// populateMaps reads the database state and populates the maps.
func (a *aliasMgr) populateMaps() error {
	// This map tracks the base SCIDs that are confirmed and don't need to
	// have entries in the *ToBase mappings as they won't be used in the
	// gossiper.
	baseConfMap := make(map[lnwire.ShortChannelID]struct{})

	// This map caches what is found in the database and is used to
	// populate the aliasMgr's actual maps.
	aliasMap := make(map[lnwire.ShortChannelID]lnwire.ShortChannelID)

	err := kvdb.Update(a.backend, func(tx kvdb.RwTx) error {
		baseConfBucket, err := tx.CreateTopLevelBucket(confirmedBucket)
		if err != nil {
			return err
		}

		err = baseConfBucket.ForEach(func(k, v []byte) error {
			// The key will the base SCID and the value will be
			// empty. Existence in the bucket means the SCID is
			// confirmed.
			baseScid := lnwire.NewShortChanIDFromInt(
				byteOrder.Uint64(k[:]),
			)
			baseConfMap[baseScid] = struct{}{}
			return nil
		})
		if err != nil {
			return err
		}

		aliasToBaseBucket, err := tx.CreateTopLevelBucket(aliasBucket)
		if err != nil {
			return err
		}

		err = aliasToBaseBucket.ForEach(func(k, v []byte) error {
			// The key will be the alias SCID and the value will be
			// the base SCID.
			aliasScid := lnwire.NewShortChanIDFromInt(
				byteOrder.Uint64(k[:]),
			)
			baseScid := lnwire.NewShortChanIDFromInt(
				byteOrder.Uint64(v[:]),
			)
			aliasMap[aliasScid] = baseScid
			return nil
		})
		return err
	}, func() {
		baseConfMap = make(map[lnwire.ShortChannelID]struct{})
		aliasMap = make(
			map[lnwire.ShortChannelID]lnwire.ShortChannelID,
		)
	})
	if err != nil {
		return err
	}

	// Populate the baseToSet map regardless if the baseSCID is marked as
	// public with 6 confirmations.
	for aliasSCID, baseSCID := range aliasMap {
		a.baseToSet[baseSCID] = append(a.baseToSet[baseSCID], aliasSCID)
	}

	// Populate the aliasToBase map if the baseSCID isn't marked as public
	// with 6 confirmations.
	for aliasSCID, baseSCID := range aliasMap {
		// Skip if baseSCID is in the baseConfMap.
		if _, ok := baseConfMap[baseSCID]; !ok {
			continue
		}

		a.aliasToBase[aliasSCID] = baseSCID
	}

	return nil
}

// addLocalAlias adds a database mapping from the passed alias to the passed
// database SCID.
func (a *aliasMgr) addLocalAlias(alias, dbScid lnwire.ShortChannelID) error {
	a.Lock()
	defer a.Unlock()

	err := kvdb.Update(a.backend, func(tx kvdb.RwTx) error {
		aliasToBaseBucket, err := tx.CreateTopLevelBucket(aliasBucket)
		if err != nil {
			return err
		}

		var (
			aliasBytes [8]byte
			baseBytes  [8]byte
		)

		byteOrder.PutUint64(aliasBytes[:], alias.ToUint64())
		byteOrder.PutUint64(baseBytes[:], dbScid.ToUint64())
		return aliasToBaseBucket.Put(aliasBytes[:], baseBytes[:])
	}, func() {})
	if err != nil {
		return err
	}

	// Update the aliasToBase and baseToSet maps.
	a.baseToSet[dbScid] = append(a.baseToSet[dbScid], alias)
	a.aliasToBase[alias] = dbScid

	return nil
}

// getAliases fetches the set of aliases stored under a given base SCID from
// write-through caches.
func (a *aliasMgr) getAliases(base lnwire.ShortChannelID) (
	[]lnwire.ShortChannelID, error) {

	a.RLock()
	defer a.RUnlock()

	aliasSet, ok := a.baseToSet[base]
	if ok {
		// Copy the found alias slice.
		setCopy := make([]lnwire.ShortChannelID, 0, len(aliasSet))
		for _, alias := range aliasSet {
			setCopy = append(setCopy, alias)
		}
		return setCopy, nil
	}

	return nil, fmt.Errorf("no aliases found for base: %v", base)
}

// findBaseSCID finds the base SCID for a given alias. This is used in the
// gossiper to find the correct SCID to lookup in the graph database.
func (a *aliasMgr) findBaseSCID(
	alias lnwire.ShortChannelID) (lnwire.ShortChannelID, error) {

	a.RLock()
	defer a.RUnlock()

	base, ok := a.aliasToBase[alias]
	if ok {
		return base, nil
	}

	return lnwire.ShortChannelID{}, errNoBase
}

// deleteSixConfs removes a mapping for the gossiper once six confirmations
// have been reached and the channel is public. At this point, only the
// confirmed SCID should be used.
func (a *aliasMgr) deleteSixConfs(baseScid lnwire.ShortChannelID) error {
	a.Lock()
	defer a.Unlock()

	err := kvdb.Update(a.backend, func(tx kvdb.RwTx) error {
		baseConfBucket, err := tx.CreateTopLevelBucket(confirmedBucket)
		if err != nil {
			return err
		}

		var baseBytes [8]byte
		byteOrder.PutUint64(baseBytes[:], baseScid.ToUint64())
		return baseConfBucket.Put(baseBytes[:], []byte{})
	}, func() {})
	if err != nil {
		return err
	}

	// Now that the database state has been updated, we'll delete all of
	// the aliasToBase mappings for this SCID.
	for alias, base := range a.aliasToBase {
		if base.ToUint64() == baseScid.ToUint64() {
			delete(a.aliasToBase, alias)
		}
	}

	return nil
}

// putPeerAlias stores the peer's alias SCID once we learn of it in the
// funding_locked message.
func (s *aliasMgr) putPeerAlias(chanID lnwire.ChannelID,
	alias lnwire.ShortChannelID) error {

	return kvdb.Update(s.backend, func(tx kvdb.RwTx) error {
		bucket, err := tx.CreateTopLevelBucket(invoiceAliasBucket)
		if err != nil {
			return err
		}

		var scratch [8]byte
		byteOrder.PutUint64(scratch[:], alias.ToUint64())
		return bucket.Put(chanID[:], scratch[:])
	}, func() {})
}

// getPeerAlias retrieves a peer's alias SCID by the channel's ChanID.
func (s *aliasMgr) getPeerAlias(chanID lnwire.ChannelID) (
	lnwire.ShortChannelID, error) {

	var alias lnwire.ShortChannelID

	err := kvdb.Update(s.backend, func(tx kvdb.RwTx) error {
		bucket, err := tx.CreateTopLevelBucket(invoiceAliasBucket)
		if err != nil {
			return err
		}

		aliasBytes := bucket.Get(chanID[:])
		if aliasBytes == nil {
			return nil
		}

		alias = lnwire.NewShortChanIDFromInt(
			byteOrder.Uint64(aliasBytes),
		)
		return nil
	}, func() {})

	if alias == hop.Source {
		return alias, errNoPeerAlias
	}

	return alias, err
}

// requestAlias returns a new ALIAS ShortChannelID to the caller by allocating
// the next un-allocated ShortChannelID. The starting ShortChannelID is
// 10000:0:0 and the ending ShortChannelID is 262143:16777215:65535. This gives
// roughly 2^58 possible ALIAS ShortChannelIDs which ensures this space won't
// get exhausted.
func (s *aliasMgr) requestAlias() (lnwire.ShortChannelID, error) {
	var nextAlias lnwire.ShortChannelID

	err := kvdb.Update(s.backend, func(tx kvdb.RwTx) error {
		bucket, err := tx.CreateTopLevelBucket(aliasAllocBucket)
		if err != nil {
			return err
		}

		lastBytes := bucket.Get(lastAliasKey)
		if lastBytes == nil {
			// If the key does not exist, then we can write the
			// startingAlias to it.
			nextAlias = startingAlias

			var scratch [8]byte
			byteOrder.PutUint64(scratch[:], nextAlias.ToUint64())
			return bucket.Put(lastAliasKey, scratch[:])
		}

		// Otherwise the key does exist so we can convert the retrieved
		// lastAlias to a ShortChannelID and use it to assign the next
		// ShortChannelID. This next ShortChannelID will then be
		// persisted in the database.
		lastScid := lnwire.NewShortChanIDFromInt(
			byteOrder.Uint64(lastBytes),
		)
		nextAlias = getNextScid(lastScid)

		var scratch [8]byte
		byteOrder.PutUint64(scratch[:], nextAlias.ToUint64())
		return bucket.Put(lastAliasKey, scratch[:])
	}, func() {})
	if err != nil {
		return nextAlias, err
	}

	return nextAlias, nil
}

// getNextScid is a utility function that returns the next SCID for a given
// alias SCID. The BlockHeight ranges from [10000, 262143], the TxIndex ranges
// from [1, 16777215], and the TxPosition ranges from [1, 65535].
func getNextScid(last lnwire.ShortChannelID) lnwire.ShortChannelID {
	var (
		next            lnwire.ShortChannelID
		incrementIdx    bool
		incrementHeight bool
	)

	if last.TxPosition == 65535 {
		// If the TxPosition is 65535, then it goes to 0 and we need to
		// increment the TxIndex.
		incrementIdx = true
	}

	if last.TxIndex == 16777215 && incrementIdx {
		// If the TxIndex is 16777215 and we need to increment it, then
		// it goes to 0 and we need to increment the BlockHeight.
		incrementIdx = false
		incrementHeight = true
	}

	switch {
	case incrementIdx:
		// If we increment the TxIndex, then TxPosition goes to 0.
		next.BlockHeight = last.BlockHeight
		next.TxIndex = last.TxIndex + 1
		next.TxPosition = 0
	case incrementHeight:
		// If we increment the BlockHeight, then the Tx fields go to 0.
		next.BlockHeight = last.BlockHeight + 1
		next.TxIndex = 0
		next.TxPosition = 0
	default:
		// Otherwise, we only need to increment the TxPosition.
		next.BlockHeight = last.BlockHeight
		next.TxIndex = last.TxIndex
		next.TxPosition = last.TxPosition + 1
	}

	return next
}

// isAlias returns true if the passed SCID is an alias. The function determines
// this by looking at the BlockHeight. If the BlockHeight is greater than 10000
// and less than 2^18, then it is an alias assigned by requestAlias.
func isAlias(scid lnwire.ShortChannelID) bool {
	return scid.BlockHeight >= 10000 && scid.BlockHeight < 1<<18
}
