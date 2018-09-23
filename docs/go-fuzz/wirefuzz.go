// +build gofuzz

package fuzz

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"reflect"

	"github.com/lightningnetwork/lnd/lnwire"
)

// Fuzz is used by go-fuzz to fuzz for potentially malicious input
func Fuzz(data []byte) int {
	// Because go-fuzz requires this function signature with a []byte parameter,
	// and we want to emulate the behavior of mainScenario in lnwire_test.go,
	// we first parse the []byte parameter into a Message type.

	// Create a reader with the byte array.
	r := bytes.NewReader(data)
	bufReader := bufio.NewReader(r)

	// Before deserializing the byte array into the desired message, we
	// must check that the passed byte array is less than this message's
	// max payload constraint.
	mType, err := bufReader.Peek(2)
	if err != nil {
		// Ignore this input
		return 0
	}

	msgType := lnwire.MessageType(binary.BigEndian.Uint16(mType[:]))

	// Now that we know the target message type, we can create the proper
	// empty message type and determine the message's max payload size.
	emptyMsg, err := lnwire.MakeEmptyMessage(msgType)
	if err != nil {
		// Ignore this input
		return 0
	}

	// Make sure byte array length (excluding 2 bytes for message type) is
	// less than max payload size for this specific message. We check this
	// because otherwise `go-fuzz` will keep creating inputs that crash on
	// ReadMessage due to a large message size.
	payloadLen := uint32(len(data)) - 2
	if payloadLen > emptyMsg.MaxPayloadLength(0) {
		// Ignore this input - max payload constraint violated
		return 0
	}

	msg, err := lnwire.ReadMessage(r, 0)
	if err != nil {
		// Ignore this input - go-fuzz generated []byte that cannot be represented as Message
		return 0
	}

	// We will serialize Message into a new bytes buffer
	var b bytes.Buffer
	if _, err := lnwire.WriteMessage(&b, msg, 0); err != nil {
		// Could not serialize Message into bytes buffer, panic
		panic(err)
	}

	// Deserialize the message from the serialized bytes buffer and
	// assert that the original message is equal to the newly deserialized message.
	newMsg, err := lnwire.ReadMessage(&b, 0)
	if err != nil {
		// Could not deserialize message from bytes buffer, panic
		panic(err)
	}
	if !reflect.DeepEqual(msg, newMsg) {
		// Deserialized message and original message are not deeply equal
		panic(fmt.Errorf("Deserialized message and original message " +
			"are not deeply equal."))
	}

	return 1
}
