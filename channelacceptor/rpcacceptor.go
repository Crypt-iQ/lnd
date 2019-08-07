package channelacceptor

import (
	"fmt"
	"sync"
	"time"
)

const (
	// DefaultRPCTimeout is the default time that the Accept method will
	// listen for responses from the RPC server, after which it will timeout.
	DefaultRPCTimeout = 15 * time.Second
)

// OpenChannelResponse is a struct containing the response from the RPC client
// which includes whether or not they accept the open channel request and to
// which PendingChannelID the response is to.
type OpenChannelResponse struct {
	Accept           bool
	PendingChannelID [32]byte
}

// ChanAcceptorClient represents an RPC streaming client that responds to
// OpenChannel queries issued by the RPCAcceptor.
type ChanAcceptorClient struct {
	SendChan chan *OpenChannelRequest
	RecvChan chan OpenChannelResponse

	closeChan chan struct{}
	active    bool
}

// RPCAcceptor represents the RPC-controlled variant of the ChannelAcceptor.
// Only one RPC client is currently allowed at a time.
type RPCAcceptor struct {
	started sync.Once
	stopped sync.Once

	clientMtx sync.Mutex
	client    *ChanAcceptorClient

	acceptChan    chan *OpenChannelRequest
	acceptClients map[[32]byte]chan bool

	quit chan struct{}
	wg   sync.WaitGroup
}

// Accept is a predicate on the OpenChannelRequest which is sent to the RPC
// client who will respond with the ultimate decision. This assumes a registered
// client exists.
func (r *RPCAcceptor) Accept(req *OpenChannelRequest) bool {
	// Register a new channel for the PendingChannelID associated with this
	// OpenChannelRequest.
	responseChan := make(chan bool)
	r.acceptClients[req.OpenChanMsg.PendingChannelID] = responseChan

	select {
	case r.acceptChan <- req:
	case <-r.quit:
		return false
	}

	for {
		select {
		case accept, _ := <-responseChan:
			return accept
		case <-time.After(DefaultRPCTimeout):
			return false
		case <-r.quit:
			return false
		}
	}
}

// RegisterClient registers an RPC streaming client. There may only be one RPC
// streaming client at a time.
func (r *RPCAcceptor) RegisterClient() (*ChanAcceptorClient, error) {
	r.clientMtx.Lock()
	defer r.clientMtx.Unlock()

	if r.active() {
		return nil, fmt.Errorf("client already exists")
	}

	r.client = &ChanAcceptorClient{
		SendChan:  make(chan *OpenChannelRequest),
		RecvChan:  make(chan OpenChannelResponse),
		closeChan: make(chan struct{}),
		active:    true,
	}

	return r.client, nil
}

// UnregisterClient unregisters the RPC streaming client.
func (r *RPCAcceptor) UnregisterClient() {
	r.clientMtx.Lock()
	defer r.clientMtx.Unlock()

	close(r.client.closeChan)
	r.client.active = false

	return
}

// Start starts the RPCAcceptor and allows the RPC server to register clients.
func (r *RPCAcceptor) Start() error {
	var err error
	r.started.Do(func() {
		err = r.start()
	})
	return err
}

func (r *RPCAcceptor) start() error {
	r.acceptChan = make(chan *OpenChannelRequest)
	r.acceptClients = make(map[[32]byte]chan bool)

	r.wg.Add(1)
	go r.processRequests()

	return nil
}

// Stop stops the RPCAcceptor.
func (r *RPCAcceptor) Stop() error {
	var err error
	r.stopped.Do(func() {
		err = r.stop()
	})
	return err
}

func (r *RPCAcceptor) stop() error {
	close(r.quit)
	r.wg.Wait()

	return nil
}

// processRequests receives OpenChannelRequests from the Accept method and
// forwards them to the chan between the RPC server. It also facilitates
// forwarding the OpenChannelResponses from the RPC server back to the funding
// manager.
func (r *RPCAcceptor) processRequests() {
	defer r.wg.Done()
	for {
		// If a client does not exist, continue.
		if !r.active() {
			continue
		}

		select {
		case rpcResp := <-r.client.RecvChan:
			// This is an OpenChannelResponse received from the RPC server. We
			// must forward this to the correct Accept method referenced by the
			// PendingChannelID.
			responseChan := r.acceptClients[rpcResp.PendingChannelID]

			select {
			case responseChan <- rpcResp.Accept:
			default:
				break
			}
		case chanReq := <-r.acceptChan:
			// This is an *OpenChannelRequest received from the Accept method.
			// We forward this to the RPC server.
			select {
			case r.client.SendChan <- chanReq:
			case <-r.client.closeChan:
				break
			case <-r.quit:
				return
			}
		case <-r.quit:
			return
		}
	}
}

// active lets the caller know whether or not there is currently a registered
// RPC client to accept or reject open channel requests. This function should be
// called BEFORE calling Accept so that the ChainedAcceptor can skip the
// RPCAcceptor if no client is active.
func (r *RPCAcceptor) active() bool {
	return r.client != nil && r.client.active
}

// A compile-time constraint to ensure RPCAcceptor implements the ChannelAcceptor
// interface.
var _ ChannelAcceptor = (*RPCAcceptor)(nil)
