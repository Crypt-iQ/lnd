package torsvc

import (
	"fmt"
	"net"
)

// OnionAddress contains a HiddenService
type OnionAddress struct {
	HiddenService []byte
}

// A compile time check to ensure OnionAddress implements the net.Addr
// interface.
var _ net.Addr = (*OnionAddress)(nil)

// String returns the HiddenService as a string
func (o *OnionAddress) String() string {
	return fmt.Sprintf("%s", o.HiddenService)
}

// Network returns the associated network - in this case "tcp" since Tor only
// allows connections over TCP.
func (o *OnionAddress) Network() string {
	return "tcp"
}
