// +build gofuzz

package replyshortchanidsend

import (
	fuzz "github.com/lightningnetwork/lnd/fuzz/lnwire"
	"github.com/lightningnetwork/lnd/lnwire"
)

// Fuzz is used by go-fuzz.
func Fuzz(data []byte) int {
	// Prefix with MsgReplyShortChanIDsEnd.
	data = fuzz.PrefixWithMsgType(data, lnwire.MsgReplyShortChanIDsEnd)

	// Create an empty message so that the FuzzHarness func can check
	// if the max payload constraint is violated.
	emptyMsg := lnwire.ReplyShortChanIDsEnd{}

	// Pass the message into our general fuzz harness for wire messages!
	return fuzz.Harness(data, &emptyMsg)
}
