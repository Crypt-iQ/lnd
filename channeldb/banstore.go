package channeldb

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcec"
	"github.com/coreos/bbolt"
	"sync/atomic"
)

const (
	// TODO(eugene) - defaultDbDirectory comment
	defaultDbDirectory = "genericbanstore"

	// TODO(eugene) - dbPermissions comment
	dbPermissions = 0600
)

var (
	// TODO(eugene) - bannedPeersBucket comment
	bannedPeersBucket = []byte("banned-peers")

	// TODO(eugene) - bannedKeysBucket
	bannedKeysBucket = []byte("banned-keys")

	// TODO(eugene) - bannedAddrsBucket
	bannedAddrsBucket = []byte("banned-addrs")
)

var (
	// TODO(eugene) - ErrGenericBanStoreInit comment
	ErrGenericBanStoreInit = errors.New("unable to initialize generic ban store")

	// TODO(eugene) - ErrGenericBanStoreCorrupted comment
	ErrGenericBanStoreCorrupted = errors.New("generic ban store corrupted")

	// TODO(eugene) - ErrPubKeyIsBanned comment
	ErrPubKeyIsBanned = errors.New("public key is banned")

	// TODO(eugene) - ErrAddrIsBanned comment
	ErrAddrIsBanned = errors.New("address is banned")
)

// TODO(eugene) - BanStore comment
type BanStore interface {
	BanPeer(pubkey *btcec.PublicKey, timeout time.Duration, addrs ...net.Addr) error
	UnbanNode(pubkey *btcec.PublicKey) error
	UnbanAddr(addr net.Addr) error
	IsNodeBanned(pubkey *btcec.PublicKey) error
	IsAddrBanned(addr net.Addr) error
}

// TODO(eugene) - GenericBanStore comment
type GenericBanStore struct {
	started int32 // To be used atomically.
	stopped int32 // To be used atomically.

	dbPath string

	db *bbolt.DB

	gcInterval time.Duration

	wg   sync.WaitGroup
	quit chan struct{}
}

// TODO(eugene) - NewGenericBanStore comment
func NewGenericBanStore(dbPath string, interval time.Duration) *GenericBanStore {

	// Use default path for log database
	if dbPath == "" {
		dbPath = defaultDbDirectory
	}

	return &GenericBanStore{
		dbPath:     dbPath,
		gcInterval: interval,
		quit:       make(chan struct{}),
	}
}

// TODO(eugene) - Start comment
func (g *GenericBanStore) Start() error {
	if !atomic.CompareAndSwapInt32(&g.started, 0, 1) {
		return nil
	}

	// Open the boltdb for use.
	var err error
	if g.db, err = bbolt.Open(g.dbPath, dbPermissions, nil); err != nil {
		return fmt.Errorf("Could not open boltdb: %v", err)
	}

	// Initialize the primary buckets used by the generic ban store.
	if err := g.initBuckets(); err != nil {
		return err
	}

	// Start garbageCollector.
	g.wg.Add(1)
	go g.garbageCollector()

	return nil
}

// TODO(eugene) - initBuckets comment
func (g *GenericBanStore) initBuckets() error {
	return g.db.Update(func(tx *bbolt.Tx) error {
		// First create the top-level bucket which contains both the
		// banned-keys and banned-addrs buckets.
		bannedPeers, err := tx.CreateBucketIfNotExists(bannedPeersBucket)
		if err != nil {
			return ErrGenericBanStoreInit
		}

		// Create the banned-keys bucket which houses the banned pubkeys.
		_, err = bannedPeers.CreateBucketIfNotExists(bannedKeysBucket)
		if err != nil {
			return ErrGenericBanStoreInit
		}

		// Create the banned-addrs bucket which houses the banned addrs.
		_, err = bannedPeers.CreateBucketIfNotExists(bannedAddrsBucket)
		if err != nil {
			return ErrGenericBanStoreInit
		}

		return nil
	})
}

// TODO(eugene) - Stop comment
func (g *GenericBanStore) Stop() error {
	if !atomic.CompareAndSwapInt32(&g.stopped, 0, 1) {
		return nil
	}

	// Stop garbageCollector.
	close(g.quit)

	g.wg.Wait()

	// Close db.
	g.db.Close()

	return nil
}

// TODO(eugene) - garbageCollector comment
func (g *GenericBanStore) garbageCollector() {
	defer g.wg.Done()

	gcTimer := time.NewTicker(g.gcInterval)
	defer gcTimer.Stop()

	for {
		select {
		case t := <-gcTimer.C:
			// The gc timer has ticked, which means it's time to unban nodes with
			// expired ban timers.
			numUnbanned, err := g.unbanExpiredNodes(t)
			if err != nil {
				log.Errorf("Unable to unban any nodes at time=%s", t)
			}

			if numUnbanned > 0 {
				log.Infof("Unbanned %v nodes at time=%s", numUnbanned, t)
			}

		case <-g.quit:
			// Received shutdown request.
			log.Infof("Generic ban store received shutdown request")
			return
		}
	}
}

// TODO(eugene) - unbanExpiredNodes comment
func (g *GenericBanStore) unbanExpiredNodes(t time.Time) (uint32, error) {
	var numUnbannedNodes uint32

	err := g.db.Update(func(tx *bbolt.Tx) error {
		numUnbannedNodes = 0

		// Grab the banned peers bucket
		bannedPeers := tx.Bucket(bannedPeersBucket)
		if bannedPeers == nil {
			return ErrGenericBanStoreCorrupted
		}

		bannedKeys := bannedPeers.Bucket(bannedKeysBucket)
		if bannedKeys == nil {
			return ErrGenericBanStoreCorrupted
		}

		bannedAddrs := bannedPeers.Bucket(bannedAddrsBucket)
		if bannedAddrs == nil {
			return ErrGenericBanStoreCorrupted
		}

		// Loop through all the entries in the bannedKeys bucket and get the
		// expired entries.
		var expiredKeys [][]byte
		if err := bannedKeys.ForEach(func(k, v []byte) error {
			// Deserialize the expiry timestamp for this entry
			expiry := time.Unix(0, int64(binary.BigEndian.Uint64(v)))

			// Check if this entry is expired
			if t.After(expiry) {
				expiredKeys = append(expiredKeys, k)
				numUnbannedNodes++
			}

			return nil
		}); err != nil {
			return err
		}

		// Loop through all the entries in the bannedAddrs bucket and get the
		// expired entries.
		var expiredAddrs [][]byte
		if err := bannedAddrs.ForEach(func(k, v []byte) error {
			// Deserialize the expiry timestamp for this entry
			expiry := time.Unix(0, int64(binary.BigEndian.Uint64(v)))

			// Check if this entry is expired
			if t.After(expiry) {
				expiredAddrs = append(expiredAddrs, k)
				numUnbannedNodes++
			}

			return nil
		}); err != nil {
			return err
		}

		// Delete every item of the expiredKeys array.  This must be done
		// explicitly outside of the ForEach function for safety reasons.
		for _, key := range expiredKeys {
			err := bannedKeys.Delete(key)
			if err != nil {
				return err
			}
		}

		// Delete every item of the expiredAddrs array.  This must be done
		// explicitly outside of the ForEach function for safety reasons.
		for _, addr := range expiredAddrs {
			err := bannedAddrs.Delete(addr)
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return 0, nil
	}

	return numUnbannedNodes, nil
}

// TODO(eugene) - BanPeer comment
func (g *GenericBanStore) BanPeer(pubkey *btcec.PublicKey,
	timeout time.Duration, addrs ...net.Addr) error {

	return g.db.Update(func(tx *bbolt.Tx) error {
		// Grab the top level bucket
		bannedPeers := tx.Bucket(bannedPeersBucket)
		if bannedPeers == nil {
			return ErrGenericBanStoreCorrupted
		}

		bannedKeys := bannedPeers.Bucket(bannedKeysBucket)
		if bannedKeys == nil {
			return ErrGenericBanStoreCorrupted
		}

		bannedAddrs := bannedPeers.Bucket(bannedAddrsBucket)
		if bannedAddrs == nil {
			return ErrGenericBanStoreCorrupted
		}

		// Get the current time and add the specified duration to get
		// the ban expiry time.
		expiry := time.Now().Add(timeout).UnixNano()

		// First we serialize the time at which the ban expires.
		var expiryBytes [8]byte
		binary.BigEndian.PutUint64(expiryBytes[:], uint64(expiry))

		// TODO(eugene) - If keys / addresses already exist, should we increase
		// their current ban time?

		// Check that the pubkey parameter is not nil since the caller
		// can specify either a pubkey to ban or addresses, or both.
		if pubkey != nil {
			// Ban this public key.
			err := bannedKeys.Put(pubkey.SerializeCompressed(), expiryBytes[:])
			if err != nil {
				return err
			}
		}

		var b bytes.Buffer

		// Ban all of the addresses.
		for _, addr := range addrs {
			// Serialize the current address
			if err := serializeAddr(&b, addr); err != nil {
				return err
			}

			// Ban this address.
			err := bannedAddrs.Put(b.Bytes(), expiryBytes[:])
			if err != nil {
				return err
			}

			// Reset Buffer for more serialization.
			b.Reset()
		}

		return nil
	})
}

// TODO(eugene) - UnbanNode comment
func (g *GenericBanStore) UnbanNode(pubkey *btcec.PublicKey) error {
	return g.db.Update(func(tx *bbolt.Tx) error {
		// Grab the top level bucket
		bannedPeers := tx.Bucket(bannedPeersBucket)
		if bannedPeers == nil {
			return ErrGenericBanStoreCorrupted
		}

		bannedKeys := bannedPeers.Bucket(bannedKeysBucket)
		if bannedKeys == nil {
			return ErrGenericBanStoreCorrupted
		}

		// Delete the serialized, compressed public key
		return bannedKeys.Delete(pubkey.SerializeCompressed())
	})
}

// TODO(eugene) - UnbanAddr comment
func (g *GenericBanStore) UnbanAddr(addr net.Addr) error {
	return g.db.Update(func(tx *bbolt.Tx) error {
		// Grab the top level bucket
		bannedPeers := tx.Bucket(bannedPeersBucket)
		if bannedPeers == nil {
			return ErrGenericBanStoreCorrupted
		}

		bannedAddrs := bannedPeers.Bucket(bannedAddrsBucket)
		if bannedAddrs == nil {
			return ErrGenericBanStoreCorrupted
		}

		var b bytes.Buffer

		// Serialize the address
		if err := serializeAddr(&b, addr); err != nil {
			return err
		}

		// Delete the serialized net.Addr
		return bannedAddrs.Delete(b.Bytes())
	})
}

// TODO(eugene) - IsNodeBanned comment
func (g *GenericBanStore) IsNodeBanned(pubkey *btcec.PublicKey) error {
	return g.db.View(func(tx *bbolt.Tx) error {
		// Grab the top level bucket
		bannedPeers := tx.Bucket(bannedPeersBucket)
		if bannedPeers == nil {
			return ErrGenericBanStoreCorrupted
		}

		bannedKeys := bannedPeers.Bucket(bannedKeysBucket)
		if bannedKeys == nil {
			return ErrGenericBanStoreCorrupted
		}


		bannedTime := bannedKeys.Get(pubkey.SerializeCompressed())
		if bannedTime == nil {
			return nil
		}

		return ErrPubKeyIsBanned
	})
}

// TODO(eugene) - IsAddrBanned comment
func (g *GenericBanStore) IsAddrBanned(addr net.Addr) error {
	return g.db.View(func(tx *bbolt.Tx) error {
		// Grab the top level bucket
		bannedPeers := tx.Bucket(bannedPeersBucket)
		if bannedPeers == nil {
			return ErrGenericBanStoreCorrupted
		}

		bannedAddrs := bannedPeers.Bucket(bannedAddrsBucket)
		if bannedAddrs == nil {
			return ErrGenericBanStoreCorrupted
		}

		var b bytes.Buffer

		// Serialize the address
		if err := serializeAddr(&b, addr); err != nil {
			return err
		}

		bannedTime := bannedAddrs.Get(b.Bytes())
		if bannedTime == nil {
			return nil
		}

		return ErrAddrIsBanned
	})
}

// TODO(eugene) - compile time check comment
var _ BanStore = (*GenericBanStore)(nil)
