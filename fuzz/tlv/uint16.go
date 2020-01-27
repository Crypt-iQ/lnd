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
		val  uint16
		val2 uint16
		buf  [8]byte
		b    bytes.Buffer
	)

	if err := lndtlv.DUint16(r, &val, &buf, 2); err != nil {
		return -1
	}

	if err := lndtlv.EUint16(&b, &val, &buf); err != nil {
		return 0
	}

	if !bytes.Equal(b.Bytes(), data) {
		panic("bytes not equal")
	}

	r2 := bytes.NewReader(b.Bytes())

	if err := lndtlv.DUint16(r2, &val2, &buf, 2); err != nil {
		return 0
	}

	if val != val2 {
		panic("values not equal")
	}

	return 1
}
