package htlcswitch

import (
	"encoding/binary"

	"github.com/lightningnetwork/lnd/kvdb"
	"github.com/lightningnetwork/lnd/lnwire"
)

var (
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
)

// aliasStore is a struct that has an underlying database and uses it to
// allocate alias ShortChannelIDs. It only stores the last allocated alias. It
// is also capable of storing the remote peer's alias SCIDs for
// option_scid_alias channels.
type aliasStore struct {
	backend kvdb.Backend
}

func newAliasStore(db kvdb.Backend) *aliasStore {
	return &aliasStore{backend: db}
}

// putPeerAlias stores the peer's alias SCID once we learn of it in the
// funding_locked message.
func (s *aliasStore) putPeerAlias(chanID lnwire.ChannelID,
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
func (s *aliasStore) getPeerAlias(chanID lnwire.ChannelID) (
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

	return alias, err
}

// requestAlias returns a new ALIAS ShortChannelID to the caller by allocating
// the next un-allocated ShortChannelID. The starting ShortChannelID is
// 10000:0:0 and the ending ShortChannelID is 262143:16777215:65535. This gives
// roughly 2^58 possible ALIAS ShortChannelIDs which ensures this space won't
// get exhausted.
func (s *aliasStore) requestAlias() (lnwire.ShortChannelID, error) {
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

// IsAlias returns true if the passed SCID is an alias. The function determines
// this by looking at the BlockHeight. If the BlockHeight is greater than 10000
// and less than 2^18, then it is an alias assigned by requestAlias.
func IsAlias(scid lnwire.ShortChannelID) bool {
	return scid.BlockHeight >= 10000 && scid.BlockHeight < 1<<18
}
