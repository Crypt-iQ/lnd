package channeldb

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcec"
	"github.com/coreos/bbolt"
)

const (
	// TODO(eugene) - defaultDbDirectory comment
	defaultDbDirectory = "genericbanstore"

	// TODO(eugene) - dbPermissions comment
	dbPermissions = 0600

	// TODO(eugene) - defaultGcInterval comment
	defaultGcInterval = 60 * 1000
)

var (
	// TODO(eugene) - bannedPeersBucket comment
	bannedPeersBucket = []byte("banned-peers")
)

var (
	// TODO(eugene) - ErrGenericBanStoreInit comment
	ErrGenericBanStoreInit = errors.New("unable to initialize generic ban store")
)

// TODO(eugene) - BanStore comment
type BanStore interface {
	BanNode(pubkey *btcec.PublicKey, timeout time.Duration, addrs ...net.Addr) error
	UnbanNode(pubkey *btcec.PublicKey) error
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
func NewGenericBanStore(dbPath string) *GenericBanStore {

	// Use default path for log database
	if dbPath == "" {
		dbPath = defaultDbDirectory
	}

	return &GenericBanStore{
		dbPath:     dbPath,
		gcInterval: defaultGcInterval,
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
func (g *GenericBanStore) initBuckets error {
	return g.db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bannedPeersBucket)
		if err != nil {
			return ErrGenericBanStoreInit
		}

		return nil
	})
}

// TODO(eugene) - Stop comment
func (g *GenericBanStore) Stop() error {
	if !atomic.CompareAndSwapInt32(&d.stopped, 0, 1) {
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

	gcTimer := time.NewTicker(gcInterval)
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
			log.Infof("Generous ban store received shutdown request")
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
			return fmt.Errorf("bannedPeersBucket is nil")
		}

		// TODO - How are we going to store this?
		// TODO - Two separate buckets for IP & PubKey? Then how would BanNode work?
		//
	})
	if err != nil {
		return 0, nil
	}

	return numUnbannedNodes, nil
}

// TODO(eugene) - BanNode comment
func (g *GenericBanStore) BanNode(pubkey *btcec.PublicKey, 
	timeout time.Duration, addrs ...net.Addr) error {
		//
}

// TODO(eugene) - UnbanNode comment
func (g *GenericBanStore) UnbanNode(pubkey *btcec.PublicKey) error {
	//
}

// TODO(eugene) - IsNodeBanned comment
func (g *GenericBanStore) IsNodeBanned(pubkey *btcec.PublicKey) error {
	//
}

// TODO(eugene) - IsAddrBanned comment
func (g *GenericBanStore) IsAddrBanned(addr net.Addr) error {
	//
}

// TODO(eugene) - compile time check comment
var _ BanStore = (*GenericBanStore)(nil)
