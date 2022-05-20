package chanacceptor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// dummyAcceptor is a ChannelAcceptor that will never return a failure.
type dummyAcceptor struct{}

func (d *dummyAcceptor) Accept(
	req *ChannelAcceptRequest) *ChannelAcceptResponse {

	return &ChannelAcceptResponse{}
}

// TestZeroConfAcceptor verifies that the ZeroConfAcceptor will deny requests
// if the internal ChainedAcceptor does not have any sub-acceptors.
func TestZeroConfAcceptor(t *testing.T) {
	t.Parallel()

	// Create the zero-conf acceptor.
	zeroAcceptor := NewZeroConfAcceptor()

	// Assert that calling Accept will return a failure.
	req := &ChannelAcceptRequest{}
	resp := zeroAcceptor.Accept(req)
	require.True(t, resp.RejectChannel())

	// Add a dummyAcceptor to the zero-conf acceptor. Assert that Accept
	// does not return a failure.
	dummy := &dummyAcceptor{}
	dummyID := zeroAcceptor.AddAcceptor(dummy)
	resp = zeroAcceptor.Accept(req)
	require.False(t, resp.RejectChannel())

	// Remove the dummyAcceptor from the zero-conf acceptor and assert that
	// Accept returns a failure.
	zeroAcceptor.RemoveAcceptor(dummyID)
	resp = zeroAcceptor.Accept(req)
	require.True(t, resp.RejectChannel())
}
