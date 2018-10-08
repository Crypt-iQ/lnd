package fuzzing

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"reflect"

	"github.com/lightningnetwork/lnd/lnwire"
)

func Fuzz(data []byte) int {
	// Because go-fuzz requires this function signature with a []byte parameter,
	// and we want to emulate the behavior of mainScenario in lnwire_test.go,
	// we first parse the []byte parameter into an Init message type.

	// Prefix with MsgInit.
	prefix := make([]byte, 2)
	binary.BigEndian.PutUint16(prefix, uint16(lnwire.MsgInit))
	data = append(prefix, data...)

	// Create a reader with the byte array.
	r := bytes.NewReader(data)

	// TODO - We can change how Global & Local FeatureVectors are processed later.
	// 	- Like the inflation bug idea.
	// TODO - Are there other things we can test besides serialization and
	// deserialization?

	// Make sure byte array length (excluding 2 bytes for message type) is
	// less than max payload size for the Init message. We check this because
	// otherwise `go-fuzz` will keep creating inputs that crash on ReadMessage
	// due to a large message size.
	emptyMsg := lnwire.Init{}
	payloadLen := uint32(len(data)) - 2
	if payloadLen > emptyMsg.MaxPayloadLength(0) {
		// Ignore this input - max payload constraint violated
		return 0
	}

	msg, err := lnwire.ReadMessage(r, 0)
	if err != nil {
		// Ignore this input - go-fuzz generated []byte that cannot be
		// represented as an Init message.
		// TODO - What about weird error messages?
		return 0
	}

	// We will serialize the Init message into a new bytes buffer.
	var b bytes.Buffer
	if _, err := lnwire.WriteMessage(&b, msg, 0); err != nil {
		// Could not serialize Init message into bytes buffer, panic
		panic(err)
	}

	// Deserialize the message from the serialized bytes buffer, and then
	// assert that the original message is equal to the newly deserialized
	// message.
	newMsg, err := lnwire.ReadMessage(&b, 0)
	if err != nil {
		// Could not deserialize Init message from bytes buffer, panic
		panic(err)
	}

	if !reflect.DeepEqual(msg, newMsg) {
		// Deserialized Init message and original Init message are not
		// deeply equal
		panic(fmt.Errorf("Deserialized message and original message " +
			"are not deeply equal."))
	}

	return 1
}
