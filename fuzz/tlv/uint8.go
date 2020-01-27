// +build gofuzz

package tlv

import (
	"bytes"

	lndtlv "github.com/lightningnetwork/lnd/tlv"
)

// FuzzUint8 is used by go-fuzz.
func FuzzUint8(data []byte) int {
	r := bytes.NewReader(data)

	var (
		val uint8
		buf [8]byte
		b   bytes.Buffer
	)

	if err := lndtlv.DUint8(r, &val, &buf, 1); err != nil {
		return -1
	}

	if err := lndtlv.EUint8(&b, &val, &buf); err != nil {
		return 0
	}

	if !bytes.Equal(b.Bytes(), data) {
		return 0
	}

	return 1
}
