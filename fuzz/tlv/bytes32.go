// +build gofuzz

package tlv

import (
	"bytes"

	lndtlv "github.com/lightningnetwork/lnd/tlv"
)

// FuzzBytes32 is used by go-fuzz.
func FuzzBytes32(data []byte) int {
	if len(data) > 32 {
		return -1
	}

	r := bytes.NewReader(data)

	var (
		val  [32]byte
		val2 [32]byte
		buf  [8]byte
		b    bytes.Buffer
	)

	if err := lndtlv.DBytes32(r, &val, &buf, 32); err != nil {
		return -1
	}

	if err := lndtlv.EBytes32(&b, &val, &buf); err != nil {
		return 0
	}

	if !bytes.Equal(b.Bytes(), data) {
		panic("bytes not equal")
	}

	r2 := bytes.NewReader(b.Bytes())

	if err := lndtlv.DBytes32(r2, &val2, &buf, 32); err != nil {
		return 0
	}

	if !bytes.Equal(val[:], val2[:]) {
		panic("values not equal")
	}

	return 1
}
