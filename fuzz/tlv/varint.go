// +build gofuzz

package tlv

import (
	"bytes"

	lndtlv "github.com/lightningnetwork/lnd/tlv"
)

// Fuzz_varint is used by go-fuzz.
func Fuzz_varint(data []byte) int {
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

	// A byte comparison isn't performed here since ReadVarInt doesn't read
	// all of the bytes from data and so b.Bytes() won't be equal to data.

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
