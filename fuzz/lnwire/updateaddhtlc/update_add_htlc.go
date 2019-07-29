// +build gofuzz

package updateaddhtlc

import (
	fuzz "github.com/lightningnetwork/lnd/fuzz/lnwire"
	"github.com/lightningnetwork/lnd/lnwire"
)

// Fuzz is used by go-fuzz.
func Fuzz(data []byte) int {
	// Prefix with MsgUpdateAddHTLC.
	data = fuzz.PrefixWithMsgType(data, lnwire.MsgUpdateAddHTLC)

	// Create an empty message so that the FuzzHarness func can check
	// if the max payload constraint is violated.
	emptyMsg := lnwire.UpdateAddHTLC{}

	// Pass the message into our general fuzz harness for wire messages!
	return fuzz.Harness(data, &emptyMsg)
}
