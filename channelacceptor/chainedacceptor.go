package channelacceptor

// ChainedAcceptor represents a conjunction of ChannelAcceptor results.
type ChainedAcceptor struct {
	// Acceptors is a slice of ChannelAcceptors that will be evaluated when
	// the ChainedAcceptor's Accept method is called.
	Acceptors []ChannelAcceptor
}

// Accept evaluates the results of all Acceptors and returns the conjunction
// of all of these predicates.
func (c *ChainedAcceptor) Accept(req *OpenChannelRequest) bool {
	result := true

	for _, acceptor := range c.Acceptors {
		switch a := acceptor.(type) {
		case *DefaultAcceptor:
			result = result && a.Accept(req)
		case *RPCAcceptor:
			if !a.active() {
				break
			}

			result = result && a.Accept(req)
		default:
			// Return false if ChainedAcceptor contains an unknown acceptor.
			return false
		}
	}

	return result
}

// A compile-time constraint to ensure ChainedAcceptor implements the
// ChannelAcceptor interface.
var _ ChannelAcceptor = (*ChainedAcceptor)(nil)
