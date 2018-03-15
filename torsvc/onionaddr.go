package torsvc

import (
	"encoding/base32"
	"net"
	"strconv"
)

const (
	// base32Alphabet is the alphabet that the base32 library will use for
	// encoding and decoding v2 and v3 onion addresses.
	base32Alphabet = "abcdefghijklmnopqrstuvwxyz234567"

	// V2OnionLengthNoSuffix represents the base32-encoded v2 onion address
	// length WITHOUT the ".onion" suffix.
	V2OnionLengthNoSuffix = 16

	// V2OnionLengthSuffix represents the base32-encoded v2 onion address
	// length WITH the ".onion" suffix.
	V2OnionLengthSuffix   = 22

	// V3OnionLengthNoSuffix represents the base32-encoded v3 onion address
	// length WITHOUT the ".onion" suffix.
	V3OnionLengthNoSuffix = 56

	// V3OnionLengthSuffix represents the base32-encoded v3 onion address
	// length WITH the ".onion" suffix.
	V3OnionLengthSuffix   = 62
)

var (
	// Base32Encoding represents a base32-encoding compliant with Tor's
	// base32 encoding scheme for v2 and v3 hidden services.
	Base32Encoding = base32.NewEncoding(base32Alphabet)
)

// OnionAddress is a struct housing a hidden service (v2 & v3) as well as the
// Virtual Port that this hidden service can be reached at.
type OnionAddress struct {
	HiddenService string
	Port          int
}

// A compile-time check to ensure that OnionAddress implements the net.Addr
// interface.
var _ net.Addr = (*OnionAddress)(nil)

// String returns a string version of OnionAddress
func (o *OnionAddress) String() string {
	return net.JoinHostPort(o.HiddenService, strconv.Itoa(o.Port))
}

// Network returns the network that this implementation of net.Addr will use.
// In this case, because Tor only allows "tcp", the network is "tcp".
func (o *OnionAddress) Network() string {
	// Tor only allows "tcp".
	return "tcp"
}
