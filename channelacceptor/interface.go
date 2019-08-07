package channelacceptor

import (
	"github.com/btcsuite/btcd/btcec"
	"github.com/lightningnetwork/lnd/lnwire"
)

// OpenChannelRequest is a struct containing the requesting node's public key
// along with the lnwire.OpenChannel message that they sent when requesting an
// inbound channel. This information is provided to each acceptor so that they
// can each leverage their own decision-making with this information.
type OpenChannelRequest struct {
	Node        *btcec.PublicKey
	OpenChanMsg *lnwire.OpenChannel
}

// ChannelAcceptor is an interface that represents a predicate on the data
// contained in OpenChannelRequest.
type ChannelAcceptor interface {
	Accept(req *OpenChannelRequest) bool
}
