package channeldb

import (
	"net"
	"time"

	"github.com/btcsuite/btcd/btcec"
)

// TODO(eugene) - BanStore comment
type BanStore interface {
	BanNode(pubkey *btcec.PublicKey, timeout time.Duration, addrs ...net.Addr) error
	UnbanNode(pubkey *btcec.PublicKey) error
	IsNodeBanned(pubkey *btcec.PublicKey) error
	IsAddrBanned(addr net.Addr) error
}

// impl here?
