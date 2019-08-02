package lnd

import (
	"fmt"
	"sync"

	"github.com/btcsuite/btcd/btcec"
	"github.com/lightningnetwork/lnd/lnwire"
)

// ChannelAcceptor is an interface that represents a predicate on the
// OpenChannel message and the peer who sent the message.
type ChannelAcceptor interface {
	Accept(node *btcec.PublicKey, req *lnwire.OpenChannel) bool
}

// DefaultChannelAcceptor implements the ChannelAcceptor interface
// and will be used as the default in the funding manager.
type DefaultChannelAcceptor struct {
	clientMtx sync.Mutex
	client    *ChanAcceptorClient
}

// OpenChannelRequest is a struct containing the requesting node's
// public key along with the lnwire.OpenChannel message that they
// sent when requesting to open an inbound channel. This is sent
// to the RPC streaming client.
type OpenChannelRequest struct {
	node        *btcec.PublicKey
	openChanMsg *lnwire.OpenChannel
}

// ChanAcceptorClient represents an RPC streaming client that responds
// to OpenChannel queries issued by the DefaultChannelAcceptor.
type ChanAcceptorClient struct {
	sendChan chan *OpenChannelRequest
	recvChan chan bool
	active   bool
}

// Accept returns true if no RPC streaming client is specified.
// Otherwise, if a client is specified, the node's public key along with
// the OpenChannel message are sent to the client who will make their own
// decision and return a bool.
func (d *DefaultChannelAcceptor) Accept(node *btcec.PublicKey,
	req *lnwire.OpenChannel) bool {

	// If the client is not nil, proxy the arguments to Accept to the RPC
	// client.
	if d.client != nil && d.client.active {
		chanReq := &OpenChannelRequest{
			node:        node,
			openChanMsg: req,
		}

		select {
		case d.client.sendChan <- chanReq:
		default:
			// Could not send the OpenChannelRequest, so we return false.
			return false
		}

		// Now we wait for a response. If the channel is closed, return false.
		for {
			select {
			case accept, ok := <-d.client.recvChan:
				if !ok {
					return false
				}

				return accept
			}
		}
	}

	return true
}

// RegisterClient registers an RPC streaming client. There may only be one
// RPC streaming client at a time.
func (d *DefaultChannelAcceptor) RegisterClient() (*ChanAcceptorClient, error) {
	d.clientMtx.Lock()
	defer d.clientMtx.Unlock()

	if d.client != nil && d.client.active {
		return nil, fmt.Errorf("client already exists")
	}

	d.client = &ChanAcceptorClient{
		sendChan: make(chan *OpenChannelRequest),
		recvChan: make(chan bool),
		active:   true,
	}

	return d.client, nil
}

// UnregisterClient unregisters the RPC streaming client.
func (d *DefaultChannelAcceptor) UnregisterClient() {

	d.clientMtx.Lock()
	defer d.clientMtx.Unlock()

	d.client.active = false

	return
}

// A compile-time constraint to ensure DefaultChannelAcceptor implements
// the ChannelAcceptor interface.
var _ ChannelAcceptor = (*DefaultChannelAcceptor)(nil)
