// +build gofuzz

package tlv

import (
	"bytes"

	lndtlv "github.com/lightningnetwork/lnd/tlv"
)

// FuzzVarInt ...
func FuzzVarInt(data []byte) int {
	r := bytes.NewReader(data)

	var (
		val  uint64
		val2 uint64
		err  error
		buf  [8]byte
		b    bytes.Buffer
	)

	val, err = lndtlv.ReadVarInt(r, &buf)
	if err != nil {
		return -1
	}

	if err := lndtlv.WriteVarInt(&b, val, &buf); err != nil {
		return 0
	}

	if !bytes.Equal(b.Bytes(), data) {
		panic("bytes not equal")
	}

	r2 := bytes.NewReader(b.Bytes())

	val2, err = lndtlv.ReadVarInt(r2, &buf)
	if err != nil {
		return 0
	}

	if val != val2 {
		panic("values not equal")
	}

	return 1
}
