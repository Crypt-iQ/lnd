package tlv_test

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	"github.com/lightningnetwork/lnd/tlv"
)

type parsedTypeTest struct {
	name           string
	encode         []tlv.Type
	decode         []tlv.Type
	expParsedTypes tlv.TypeMap
}

// TestMem tests how much memory is allocated when forcing the stream to decode
// a lot of single-byte values. Buffers are usually grown to larger than their
// assigned value in some functions so we should get some MB even with a max
// payload length of 65KB!
func TestMem(t *testing.T) {
	// Create a new stream from []Record.
	decRecords := make([]tlv.Record, 0, 1)
	decRecords = append(decRecords, tlv.MakePrimitiveRecord(10001, new(uint64)))
	decStream := tlv.MustNewStream(decRecords...)

	// Create a serialized stream.
	encRecords := make([]tlv.Record, 0, 10000)
	for i := 0; i < 10000; i++ {
		encRecords = append(encRecords, tlv.MakePrimitiveRecord(tlv.Type(i), new(uint16)))
	}

	// One large payload allocates less memory than the above
	// encRecords := make([]tlv.Record, 0, 1)
	// var a [65530]byte
	// var c []byte
	// c = a[:]
	// encRecords = append(encRecords, tlv.MakePrimitiveRecord(tlv.Type(0), &c))

	encStream := tlv.MustNewStream(encRecords...)
	var b bytes.Buffer
	if err := encStream.Encode(&b); err != nil {
		panic(err)
	}

	// Size of payload
	fmt.Println(len(b.Bytes()))

	// Big alloc decode
	_, err := decStream.DecodeWithParsedTypes(
		bytes.NewReader(b.Bytes()),
	)

	// Low alloc decode
	// err := decStream.Decode(
	// 	bytes.NewReader(b.Bytes()),
	// )
	if err != nil {
		panic(err)
	}
}

// TestParsedTypes asserts that a Stream will properly return the set of types
// that it encounters when the type is known-and-decoded or unknown-and-ignored.
func TestParsedTypes(t *testing.T) {
	const (
		knownType       = 1
		unknownType     = 3
		secondKnownType = 4
	)

	tests := []parsedTypeTest{
		{
			name:   "known and unknown",
			encode: []tlv.Type{knownType, unknownType},
			decode: []tlv.Type{knownType},
			expParsedTypes: tlv.TypeMap{
				unknownType: []byte{0, 0, 0, 0, 0, 0, 0, 0},
				knownType:   nil,
			},
		},
		{
			name:   "known and missing known",
			encode: []tlv.Type{knownType},
			decode: []tlv.Type{knownType, secondKnownType},
			expParsedTypes: tlv.TypeMap{
				knownType: nil,
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			testParsedTypes(t, test)
		})
	}
}

func testParsedTypes(t *testing.T, test parsedTypeTest) {
	encRecords := make([]tlv.Record, 0, len(test.encode))
	for _, typ := range test.encode {
		encRecords = append(
			encRecords, tlv.MakePrimitiveRecord(typ, new(uint64)),
		)
	}

	decRecords := make([]tlv.Record, 0, len(test.decode))
	for _, typ := range test.decode {
		decRecords = append(
			decRecords, tlv.MakePrimitiveRecord(typ, new(uint64)),
		)
	}

	// Construct a stream that will encode the test's set of types.
	encStream := tlv.MustNewStream(encRecords...)

	var b bytes.Buffer
	if err := encStream.Encode(&b); err != nil {
		t.Fatalf("unable to encode stream: %v", err)
	}

	// Create a stream that will parse a subset of the test's types.
	decStream := tlv.MustNewStream(decRecords...)

	parsedTypes, err := decStream.DecodeWithParsedTypes(
		bytes.NewReader(b.Bytes()),
	)
	if err != nil {
		t.Fatalf("error decoding: %v", err)
	}
	if !reflect.DeepEqual(parsedTypes, test.expParsedTypes) {
		t.Fatalf("error mismatch on parsed types")
	}
}
