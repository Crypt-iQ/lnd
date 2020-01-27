// +build gofuzz

package tlv

import (
	"bytes"

	lndtlv "github.com/lightningnetwork/lnd/tlv"
)

// FuzzUint64 is used by go-fuzz.
func FuzzUint64(data []byte) int {
	if len(data) > 8 {
		return -1
	}

	r := bytes.NewReader(data)

	var (
		val  uint64
		val2 uint64
		buf  [8]byte
		b    bytes.Buffer
	)

	if err := lndtlv.DUint64(r, &val, &buf, 8); err != nil {
		return -1
	}

	if err := lndtlv.EUint64(&b, &val, &buf); err != nil {
		return 0
	}

	if !bytes.Equal(b.Bytes(), data) {
		panic("bytes not equal")
	}

	r2 := bytes.NewReader(b.Bytes())

	if err := lndtlv.DUint64(r2, &val2, &buf, 8); err != nil {
		return 0
	}

	if val != val2 {
		panic("values not equal")
	}

	return 1
}
