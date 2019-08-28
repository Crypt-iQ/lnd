package features

import (
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/lightningnetwork/lnd/zpay32"
)

// Fuzz ...
func Fuzz(data []byte) int {
	decodedInvoice := &zpay32.Invoice{}
	net := &chaincfg.MainNetParams

	// 8 hours - nothing, figure something else out
	if err := zpay32.ParseD(decodedInvoice, data, net); err != nil {
		return 0
	}

	return 1
}
