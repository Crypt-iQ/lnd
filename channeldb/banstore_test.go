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
	gcInterval = time.Millisecond * 100

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
	t.Parallel()

	dbPath := tempBanStorePath(t)

	g, err := startup(dbPath, gcInterval)
	if err != nil {
		t.Fatalf("Unable to startup GenericBanStore: %v", err)
	}
	defer shutdown(dbPath, g)

	err = g.BanPeer(testPubKey, time.Millisecond * 100, testIPV4Addr,
		testIPV6Addr, testOnionV2Addr, testOnionV3Addr)
	if err != nil {
		t.Fatalf("Unable to ban peer: %v", err)
	}

	// Wait for both database write and for GC to unban the peer.
	time.Sleep(time.Second)

	// Assert that neither the pubkey nor the addresses are banned
	err = g.IsNodeBanned(testPubKey)
	if err != nil {
		t.Fatalf("Failed to unban peer")
	}

	err = g.IsAddrBanned(testIPV4Addr)
	if err != nil {
		t.Fatalf("Failed to unban peer")
	}

	err = g.IsAddrBanned(testIPV6Addr)
	if err != nil {
		t.Fatalf("Failed to unban peer")
	}

	err = g.IsAddrBanned(testOnionV2Addr)
	if err != nil {
		t.Fatalf("Failed to unban peer")
	}

	err = g.IsAddrBanned(testOnionV3Addr)
	if err != nil {
		t.Fatalf("Failed to unban peer")
	}
}
