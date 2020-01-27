// +build gofuzz

package tlv

import (
	"bytes"
	"reflect"

	"github.com/btcsuite/btcd/btcec"
	lndtlv "github.com/lightningnetwork/lnd/tlv"
)

func FuzzStream(data []byte) int {
	var (
		val  uint8
		val2 uint16
		val3 uint32
		val4 uint64
		val5 [32]byte
		val6 [33]byte
		val7 *btcec.PublicKey
		val8 [64]byte
		val9 []byte
		b    bytes.Buffer

		val10 uint8
		val11 uint16
		val12 uint32
		val13 uint64
		val14 [32]byte
		val15 [33]byte
		val16 *btcec.PublicKey
		val17 [64]byte
		val18 []byte
	)

	stream := lndtlv.MustNewStream(
		lndtlv.MakePrimitiveRecord(1, &val),
		lndtlv.MakePrimitiveRecord(2, &val2),
		lndtlv.MakePrimitiveRecord(3, &val3),
		lndtlv.MakePrimitiveRecord(4, &val4),
		lndtlv.MakePrimitiveRecord(5, &val5),
		lndtlv.MakePrimitiveRecord(6, &val6),
		lndtlv.MakePrimitiveRecord(7, &val7),
		lndtlv.MakePrimitiveRecord(8, &val8),
		lndtlv.MakePrimitiveRecord(9, &val9),
	)

	r := bytes.NewReader(data)

	if err := stream.Decode(r); err != nil {
		return -1
	}

	if err := stream.Encode(&b); err != nil {
		return 0
	}

	if !bytes.Equal(b.Bytes(), data) {
		panic("bytes not equal")
	}

	stream2 := lndtlv.MustNewStream(
		lndtlv.MakePrimitiveRecord(1, &val10),
		lndtlv.MakePrimitiveRecord(2, &val11),
		lndtlv.MakePrimitiveRecord(3, &val12),
		lndtlv.MakePrimitiveRecord(4, &val13),
		lndtlv.MakePrimitiveRecord(5, &val14),
		lndtlv.MakePrimitiveRecord(6, &val15),
		lndtlv.MakePrimitiveRecord(7, &val16),
		lndtlv.MakePrimitiveRecord(8, &val17),
		lndtlv.MakePrimitiveRecord(9, &val18),
	)

	r2 := bytes.NewReader(b.Bytes())

	if err := stream.Decode(r2); err != nil {
		return 0
	}

	if !reflect.DeepEqual(stream, stream2) {
		panic("values not equal")
	}

	return 1
}
