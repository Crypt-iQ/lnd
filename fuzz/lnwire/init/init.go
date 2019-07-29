// +build gofuzz

package init

import (
	fuzz "github.com/lightningnetwork/lnd/fuzz/lnwire"
	"github.com/lightningnetwork/lnd/lnwire"
)

// Fuzz is used by go-fuzz.
func Fuzz(data []byte) int {
	// TODO - Since we prefix the data with 16 (the Init message prefix), should we
	// remove the prefix from corpus/Init.txt?

	// Prefix with MsgInit.
	data = fuzz.PrefixWithMsgType(data, lnwire.MsgInit)

	// Create an empty message so that the FuzzHarness func can check
	// if the max payload constraint is violated.
	emptyMsg := lnwire.Init{}

	// Pass the message into our general fuzz harness for wire messages!
	return fuzz.Harness(data, &emptyMsg)
}
