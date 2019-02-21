package channeldb

import (
	"github.com/lightningnetwork/lnd/tor"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec"
)

const (
	//
)

var (
	gcInterval = time.Second * 2

	banDuration = time.Millisecond * 100

	testPrivKeyBytes = [32]byte{
		0xb7, 0x94, 0x38, 0x5f, 0x2d, 0x1e, 0xf7, 0xab,
		0x4d, 0x92, 0x73, 0xd1, 0x90, 0x63, 0x81, 0xb4,
		0x4f, 0x2f, 0x6f, 0x25, 0x88, 0xa3, 0xef, 0xb9,
		0x6a, 0x49, 0x18, 0x83, 0x31, 0x98, 0x47, 0x53,
	}

	_, testPubKey = btcec.PrivKeyFromBytes(btcec.S256(), testPrivKeyBytes[:])

	testIPV4Addr = &net.TCPAddr{
		IP:   testIP4,
		Port: 12345,
	}

	testIPV6Addr = &net.TCPAddr{
		IP:   testIP6,
		Port: 65222,
	}

	testOnionV2Addr = &tor.OnionAddr{
		OnionService: "3g2upl4pq6kufc4m.onion",
		Port:         9735,
	}

	testOnionV3Addr = &tor.OnionAddr{
		OnionService: "vww6ybal4bd7szmgncyruucpgfkqahzddi37ktceo3ah7ngmcopnpyyd.onion",
		Port:         80,
	}
)

// TODO(eugene) - tempBanStorePath comment
func tempBanStorePath(t *testing.T) string {
	dir, err := ioutil.TempDir("", "banstore")
	if err != nil {
		t.Fatalf("unable to create temporary generic ban store dir: %v", err)
	}

	return filepath.Join(dir, "generic.db")
}

// TODO(eugene) - startup comment
func startup(dbPath string, interval time.Duration) (*GenericBanStore, error) {
	// Create the GenericBanStore object
	store := NewGenericBanStore(dbPath, interval)

	// Open the database connection and start the garbage collector
	err := store.Start()
	if err != nil {
		return nil, err
	}

	return store, nil
}

// TODO(eugene) - shutdown comment
func shutdown(dir string, g *GenericBanStore) {
	g.Stop()
	os.RemoveAll(dir)
}

// TODO(eugene) - Test comment
func TestGenericBanStoreGarbageCollector(t *testing.T) {
	dbPath := tempBanStorePath(t)

	g, err := startup(dbPath, gcInterval)
	if err != nil {
		t.Fatalf("Unable to startup GenericBanStore: %v", err)
	}
	defer shutdown(dbPath, g)

	// Ban a pubkey and various addresses
	err = g.BanPeer(testPubKey, banDuration, testIPV4Addr, testIPV6Addr,
		testOnionV2Addr, testOnionV3Addr)
	if err != nil {
		t.Fatalf("Unable to ban peer: %v", err)
	}

	// Wait for database write
	time.Sleep(time.Millisecond * 500)

	// Assert that the pubkey and addresses are banned
	err = g.IsNodeBanned(testPubKey)
	if err != ErrPubKeyIsBanned {
		t.Fatalf("Failed to ban pubkey")
	}

	err = g.IsAddrBanned(testIPV4Addr)
	if err != ErrAddrIsBanned {
		t.Fatal("Failed to ban v4 addr")
	}

	err = g.IsAddrBanned(testIPV6Addr)
	if err != ErrAddrIsBanned {
		t.Fatalf("Failed to ban v6 addr")
	}

	err = g.IsAddrBanned(testOnionV2Addr)
	if err != ErrAddrIsBanned {
		t.Fatalf("Failed to ban onion v2 addr")
	}

	err = g.IsAddrBanned(testOnionV3Addr)
	if err != ErrAddrIsBanned {
		t.Fatalf("Failed to ban onion v3 addr")
	}

	// Wait for GC to unban entries
	time.Sleep(time.Second * 5)

	// Assert that neither the pubkey nor the addresses are banned
	err = g.IsNodeBanned(testPubKey)
	if err != nil {
		t.Fatalf("Failed to unban pubkey")
	}

	err = g.IsAddrBanned(testIPV4Addr)
	if err != nil {
		t.Fatalf("Failed to unban v4 addr")
	}

	err = g.IsAddrBanned(testIPV6Addr)
	if err != nil {
		t.Fatalf("Failed to unban v6 addr")
	}

	err = g.IsAddrBanned(testOnionV2Addr)
	if err != nil {
		t.Fatalf("Failed to unban onion v2 addr")
	}

	err = g.IsAddrBanned(testOnionV3Addr)
	if err != nil {
		t.Fatalf("Failed to unban onion v3 addr")
	}
}

// TODO(eugene) - TestGenericBanStorePersistentGC
func TestGenericBanStorePersistentGC(t *testing.T) {
	dbPath := tempBanStorePath(t)

	g, err := startup(dbPath, gcInterval)
	if err != nil {
		t.Fatalf("Unable to startup GenericBanStore: %v", err)
	}
	defer shutdown(dbPath, g)

	// Ban a pubkey and an address
	err = g.BanPeer(testPubKey, banDuration, testIPV4Addr)
	if err != nil {
		t.Fatalf("Unable to ban peer: %v", err)
	}

	// Wait for database write
	time.Sleep(time.Millisecond * 500)

	// Check that both the pubkey and the address are still banned
	err = g.IsNodeBanned(testPubKey)
	if err != ErrPubKeyIsBanned {
		t.Fatalf("Failed to ban pubkey")
	}

	err = g.IsAddrBanned(testIPV4Addr)
	if err != ErrAddrIsBanned {
		t.Fatalf("Failed to ban v4 addr")
	}

	// Shut down the GenericBanStore and the garbage collector
	g.Stop()

	g2, err := startup(dbPath, gcInterval)
	if err != nil {
		t.Fatalf("Unable to startup GenericBanStore: %v", err)
	}
	defer shutdown(dbPath, g2)

	// Check that both the pubkey and the address are still banned
	err = g2.IsNodeBanned(testPubKey)
	if err != ErrPubKeyIsBanned {
		t.Fatalf("Failed to ban pubkey")
	}

	err = g2.IsAddrBanned(testIPV4Addr)
	if err != ErrAddrIsBanned {
		t.Fatalf("Failed to ban v4 addr")
	}

	// Wait for GC to unban entries
	time.Sleep(time.Second * 5)

	// Assert that both the pubkey and the address are unbanned
	err = g2.IsNodeBanned(testPubKey)
	if err != nil {
		t.Fatalf("Failed to unban pubkey")
	}

	err = g2.IsAddrBanned(testIPV4Addr)
	if err != nil {
		t.Fatalf("Failed to unban v4 addr")
	}
}

// TODO(eugene) - TestGenericBanStoreStartAndStop
func TestGenericBanStoreStartAndStop(t *testing.T) {
	dbPath := tempBanStorePath(t)

	g, err := startup(dbPath, gcInterval)
	if err != nil {
		t.Fatalf("Unable to startup GenericBanStore: %v", err)
	}
	defer shutdown(dbPath, g)

	// Ban a pubkey and an address
	err = g.BanPeer(testPubKey, banDuration, testIPV4Addr)
	if err != nil {
		t.Fatalf("Unable to ban peer: %v", err)
	}

	// Shut down the generic ban store and the garbage collector
	g.Stop()

	g2, err := startup(dbPath, gcInterval)
	if err != nil {
		t.Fatalf("Unable to startup GenericBanStore: %v", err)
	}
	defer shutdown(dbPath, g2)

	// Check that both the pubkey and address are still banned.
	err = g2.IsNodeBanned(testPubKey)
	if err != ErrPubKeyIsBanned {
		t.Fatalf("Failed to persist banned pubkey")
	}

	err = g2.IsAddrBanned(testIPV4Addr)
	if err != ErrAddrIsBanned {
		t.Fatalf("Failed to persist banned v4 addr")
	}
}

// TODO(eugene) - TestGenericBanStoreBanTimes
func TestGenericBanStoreBanTimes(t *testing.T) {
	dbPath := tempBanStorePath(t)

	g, err := startup(dbPath, gcInterval)
	if err != nil {
		t.Fatalf("Unable to startup GenericBanStore: %v", err)
	}
	defer shutdown(dbPath, g)

	// Ban a pubkey
	err = g.BanPeer(testPubKey, banDuration)
	if err != nil {
		t.Fatalf("Unable to ban peer: %v", err)
	}

	// Ban an address with a longer ban duration (5 seconds)
	err = g.BanPeer(nil, time.Second * 5, testIPV4Addr)
	if err != nil {
		t.Fatalf("Unable to ban peer: %v", err)
	}

	// Wait for database write
	time.Sleep(time.Millisecond * 500)

	// Check that both the pubkey and address are banned
	err = g.IsNodeBanned(testPubKey)
	if err != ErrPubKeyIsBanned {
		t.Fatalf("Failed to ban pubkey")
	}

	err = g.IsAddrBanned(testIPV4Addr)
	if err != ErrAddrIsBanned {
		t.Fatalf("Failed to ban v4 addr")
	}

	// Wait for GC to unban the pubkey
	time.Sleep(time.Second * 2)

	// Check that the pubkey is unbanned and the address is banned
	err = g.IsNodeBanned(testPubKey)
	if err != nil {
		t.Fatalf("Failed to unban pubkey")
	}

	err = g.IsAddrBanned(testIPV4Addr)
	if err != ErrAddrIsBanned {
		t.Fatalf("Failed to ban v4 addr")
	}

	// Wait for GC to unban the address
	time.Sleep(time.Second * 5)
	err = g.IsAddrBanned(testIPV4Addr)
	if err != nil {
		t.Fatalf("Failed to unban v4 addr")
	}
}
