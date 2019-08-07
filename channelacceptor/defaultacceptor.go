package channelacceptor

// DefaultAcceptor represents the default ChannelAcceptor. It is used when no
// other ChannelAcceptors are specified.
type DefaultAcceptor struct{}

// Accept always returns true since this is the default variant.
func (d *DefaultAcceptor) Accept(req *OpenChannelRequest) bool {
	return true
}

// A compile-time constraint to ensure DefaultAcceptor implements the
// ChannelAcceptor interface.
var _ ChannelAcceptor = (*DefaultAcceptor)(nil)
