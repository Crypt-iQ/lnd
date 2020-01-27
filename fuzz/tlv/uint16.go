// +build gofuzz

package tlv

import (
	"bytes"

	lndtlv "github.com/lightningnetwork/lnd/tlv"
)

// FuzzUint16 is used by go-fuzz.
func FuzzUint16(data []byte) int {
	r := bytes.NewReader(data)

	var (
		val uint16
		buf [8]byte
		b   bytes.Buffer
	)

	if err := lndtlv.DUint16(r, &val, &buf, 2); err != nil {
		return -1
	}

	if err := lndtlv.EUint16(&b, &val, &buf); err != nil {
		return 0
	}

	if !bytes.Equal(b.Bytes(), data) {
		return 0
	}

	return 1
}
