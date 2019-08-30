package features

import (
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/zpay32"
)

// TODO - Test that sending a bunch of init, chanannouncement messages with RawFeatureVector lengths of 65KB can OOM?
// This one isn't so bad because:
// 1) not all at once
// 2) can ban misbehaving peers (would be significantly MORE requests than the invoice case)

// TestManyRouteHints ...
func TestManyRouteHints(t *testing.T) {

	var (
		testPrivKeyBytes, _     = hex.DecodeString("e126f68f7eafcc8b74f54d269fe206be715000f94dac067d1c04a8ca3b2db734")
		testPrivKey, testPubKey = btcec.PrivKeyFromBytes(btcec.S256(), testPrivKeyBytes)
		testMillisat25mBTC      = lnwire.MilliSatoshi(2500000000)
		testPaymentHash         [32]byte
		testCoffeeBeans         = "coffee beans"

		testMessageSigner = zpay32.MessageSigner{
			SignCompact: func(hash []byte) ([]byte, error) {
				sig, err := btcec.SignCompact(btcec.S256(),
					testPrivKey, hash, true)
				if err != nil {
					return nil, fmt.Errorf("can't sign the "+
						"message: %v", err)
				}
				return sig, nil
			},
		}

		testHopHintPubkeyBytes1, _ = hex.DecodeString("029e03a901b85534ff1e92c43c74431f7ce72046060fcf7a95c37e148f78c77255")
		testHopHintPubkey1, _      = btcec.ParsePubKey(testHopHintPubkeyBytes1, btcec.S256())
		testHopHintPubkeyBytes2, _ = hex.DecodeString("039e03a901b85534ff1e92c43c74431f7ce72046060fcf7a95c37e148f78c77255")
		testHopHintPubkey2, _      = btcec.ParsePubKey(testHopHintPubkeyBytes2, btcec.S256())

		testDoubleHop = []zpay32.HopHint{
			{
				NodeID:                    testHopHintPubkey1,
				ChannelID:                 0x0102030405060708,
				FeeBaseMSat:               1,
				FeeProportionalMillionths: 20,
				CLTVExpiryDelta:           3,
			},
			{
				NodeID:                    testHopHintPubkey2,
				ChannelID:                 0x030405060708090a,
				FeeBaseMSat:               2,
				FeeProportionalMillionths: 30,
				CLTVExpiryDelta:           4,
			},
		}
	)

	inv := &zpay32.Invoice{
		Net:         &chaincfg.MainNetParams,
		MilliSat:    &testMillisat25mBTC,
		Timestamp:   time.Unix(1496314658, 0),
		PaymentHash: &testPaymentHash,
		Description: &testCoffeeBeans,
		Destination: testPubKey,
	}

	var routeHints [][]zpay32.HopHint
	// Add a lot of route hints here... :)
	for i := 0; i < 10000; i++ {
		routeHints = append(routeHints, testDoubleHop)
	}

	inv.RouteHints = routeHints

	// Encode it out to a fun string :)
	res, err := inv.Encode(testMessageSigner)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	fmt.Println(res)
}

func TestDecodeManyRouteHints(t *testing.T) {
	// Can't fit that much data here since the length of the feature vector must
	// fit into 10-bits and we can't repeat feature vector tlv fields :(
	_, err := zpay32.Decode(giantInvoiceManyRouteHints, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
}

// TestHugeFeatureVector ...
func TestHugeFeatureVector(t *testing.T) {

	var (
		testPrivKeyBytes, _     = hex.DecodeString("e126f68f7eafcc8b74f54d269fe206be715000f94dac067d1c04a8ca3b2db734")
		testPrivKey, testPubKey = btcec.PrivKeyFromBytes(btcec.S256(), testPrivKeyBytes)
		testMillisat25mBTC      = lnwire.MilliSatoshi(2500000000)
		testPaymentHash         [32]byte
		testCoffeeBeans         = "coffee beans"

		testMessageSigner = zpay32.MessageSigner{
			SignCompact: func(hash []byte) ([]byte, error) {
				sig, err := btcec.SignCompact(btcec.S256(),
					testPrivKey, hash, true)
				if err != nil {
					return nil, fmt.Errorf("can't sign the "+
						"message: %v", err)
				}
				return sig, nil
			},
		}
	)

	inv := &zpay32.Invoice{
		// Net:         &chaincfg.MainNetParams,
		Net:         &chaincfg.SimNetParams,
		MilliSat:    &testMillisat25mBTC,
		Timestamp:   time.Unix(1496314658, 0),
		PaymentHash: &testPaymentHash,
		Description: &testCoffeeBeans,
		Destination: testPubKey,
	}

	// Add the excessively large feature vector to this...
	//

	// Encode it out to a fun string :)
	res, err := inv.Encode(testMessageSigner)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	fmt.Println(res)
}

func TestDecodeBigFeatureVector(t *testing.T) {
	// Can't fit that much data here since the length of the feature vector must
	// fit into 10-bits and we can't repeat feature vector tlv fields :(
	inv := "lnsb25m1pvjluezpp5qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqdq5vdhkven9v5sxyetpdeesnp4q0n326hr8v9zprg8gsvezcch06gfaqqhde2aj730yg0durunfhv669lgllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllygk4g60466w9j8vlnu5f49fmxzw3s06kxm7jgx24lzxnk34g2js5ss5t3aw9fvwelv0ek4n882hmv706zznmzc935tn58nh7znkgtksqwmazws"
	_, err := zpay32.Decode(inv, &chaincfg.SimNetParams)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
}
