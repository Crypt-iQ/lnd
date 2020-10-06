package lnd

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcutil"
	"github.com/btcsuite/btcwallet/chain"
	"github.com/btcsuite/btcwallet/wallet"
	"github.com/btcsuite/btcwallet/walletdb"
	"github.com/lightninglabs/neutrino"
	"github.com/lightninglabs/neutrino/headerfs"
	"github.com/lightningnetwork/lnd/chainreg"
	"github.com/lightningnetwork/lnd/chainntnfs"
	"github.com/lightningnetwork/lnd/chainntnfs/bitcoindnotify"
	"github.com/lightningnetwork/lnd/chainntnfs/btcdnotify"
	"github.com/lightningnetwork/lnd/chainntnfs/neutrinonotify"
	"github.com/lightningnetwork/lnd/channeldb"
	"github.com/lightningnetwork/lnd/htlcswitch"
	"github.com/lightningnetwork/lnd/input"
	"github.com/lightningnetwork/lnd/keychain"
	"github.com/lightningnetwork/lnd/lncfg"
	"github.com/lightningnetwork/lnd/lnwallet"
	"github.com/lightningnetwork/lnd/lnwallet/btcwallet"
	"github.com/lightningnetwork/lnd/lnwallet/chainfee"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/chainview"
	"github.com/lightningnetwork/lnd/tor"
)

const (
	// DefaultBitcoinMinHTLCMSat is the default smallest value htlc this
	// node will accept. This value is proposed in the channel open sequence
	// and cannot be changed during the life of the channel. It is 1 msat by
	// default to allow maximum flexibility in deciding what size payments
	// to forward.
	//
	// All forwarded payments are subjected to the min htlc constraint of
	// the routing policy of the outgoing channel. This implicitly controls
	// the minimum htlc value on the incoming channel too.
	DefaultBitcoinMinHTLCInMSat = lnwire.MilliSatoshi(1)

	// DefaultBitcoinMinHTLCOutMSat is the default minimum htlc value that
	// we require for sending out htlcs. Our channel peer may have a lower
	// min htlc channel parameter, but we - by default - don't forward
	// anything under the value defined here.
	DefaultBitcoinMinHTLCOutMSat = lnwire.MilliSatoshi(1000)

	// DefaultBitcoinBaseFeeMSat is the default forwarding base fee.
	DefaultBitcoinBaseFeeMSat = lnwire.MilliSatoshi(1000)

	// DefaultBitcoinFeeRate is the default forwarding fee rate.
	DefaultBitcoinFeeRate = lnwire.MilliSatoshi(1)

	// DefaultBitcoinTimeLockDelta is the default forwarding time lock
	// delta.
	DefaultBitcoinTimeLockDelta = 40

	DefaultLitecoinMinHTLCInMSat  = lnwire.MilliSatoshi(1)
	DefaultLitecoinMinHTLCOutMSat = lnwire.MilliSatoshi(1000)
	DefaultLitecoinBaseFeeMSat    = lnwire.MilliSatoshi(1000)
	DefaultLitecoinFeeRate        = lnwire.MilliSatoshi(1)
	DefaultLitecoinTimeLockDelta  = 576
	DefaultLitecoinDustLimit      = btcutil.Amount(54600)

	// DefaultBitcoinStaticFeePerKW is the fee rate of 50 sat/vbyte
	// expressed in sat/kw.
	DefaultBitcoinStaticFeePerKW = chainfee.SatPerKWeight(12500)

	// DefaultBitcoinStaticMinRelayFeeRate is the min relay fee used for
	// static estimators.
	DefaultBitcoinStaticMinRelayFeeRate = chainfee.FeePerKwFloor

	// DefaultLitecoinStaticFeePerKW is the fee rate of 200 sat/vbyte
	// expressed in sat/kw.
	DefaultLitecoinStaticFeePerKW = chainfee.SatPerKWeight(50000)

	// BtcToLtcConversionRate is a fixed ratio used in order to scale up
	// payments when running on the Litecoin chain.
	BtcToLtcConversionRate = 60
)

// DefaultBtcChannelConstraints is the default set of channel constraints that are
// meant to be used when initially funding a Bitcoin channel.
//
// TODO(halseth): make configurable at startup?
var DefaultBtcChannelConstraints = channeldb.ChannelConstraints{
	DustLimit:        lnwallet.DefaultDustLimit(),
	MaxAcceptedHtlcs: input.MaxHTLCNumber / 2,
}

// DefaultLtcChannelConstraints is the default set of channel constraints that are
// meant to be used when initially funding a Litecoin channel.
var DefaultLtcChannelConstraints = channeldb.ChannelConstraints{
	DustLimit:        DefaultLitecoinDustLimit,
	MaxAcceptedHtlcs: input.MaxHTLCNumber / 2,
}

// ChainControl couples the three primary interfaces lnd utilizes for a
// particular chain together. A single ChainControl instance will exist for all
// the chains lnd is currently active on.
type ChainControl struct {
	// ChainIO represents an abstraction over a source that can query the blockchain.
	ChainIO lnwallet.BlockChainIO

	// FeeEstimator is used to estimate an optimal fee for transactions important to us.
	FeeEstimator chainfee.Estimator

	// Signer is used to provide signatures over things like transactions.
	Signer input.Signer

	// KeyRing represents a set of keys that we have the private keys to.
	KeyRing keychain.SecretKeyRing

	// Wc is an abstraction over some basic wallet commands. This base set of commands
	// will be provided to the Wallet *LightningWallet raw pointer below.
	Wc lnwallet.WalletController

	// MsgSigner is used to sign arbitrary messages.
	MsgSigner lnwallet.MessageSigner

	// ChainNotifier is used to receive blockchain events that we are interested in.
	ChainNotifier chainntnfs.ChainNotifier

	// ChainView is used in the router for maintaining an up-to-date graph.
	ChainView chainview.FilteredChainView

	// Wallet is our LightningWallet that also contains the abstract Wc above. This wallet
	// handles all of the lightning operations.
	Wallet *lnwallet.LightningWallet

	// RoutingPolicy is the routing policy we have decided to use.
	RoutingPolicy htlcswitch.ForwardingPolicy

	// MinHtlcIn is the minimum HTLC we will accept.
	MinHtlcIn lnwire.MilliSatoshi
}

// NewChainControl attempts to create a ChainControl instance according
// to the parameters in the passed configuration. Currently three
// branches of ChainControl instances exist: one backed by a running btcd
// full-node, another backed by a running bitcoind full-node, and the other
// backed by a running neutrino light client instance. When running with a
// neutrino light client instance, `neutrinoCS` must be non-nil.
func NewChainControl(cfg *chainreg.Config) (*ChainControl, error) {

	// Set the RPC config from the "home" chain. Multi-chain isn't yet
	// active, so we'll restrict usage to a particular chain for now.
	homeChainConfig := cfg.Bitcoin
	if cfg.PrimaryChain() == chainreg.LitecoinChain {
		homeChainConfig = cfg.Litecoin
	}
	ltndLog.Infof("Primary chain is set to: %v",
		cfg.PrimaryChain())

	cc := &ChainControl{}

	switch cfg.PrimaryChain() {
	case chainreg.BitcoinChain:
		cc.RoutingPolicy = htlcswitch.ForwardingPolicy{
			MinHTLCOut:    cfg.Bitcoin.MinHTLCOut,
			BaseFee:       cfg.Bitcoin.BaseFee,
			FeeRate:       cfg.Bitcoin.FeeRate,
			TimeLockDelta: cfg.Bitcoin.TimeLockDelta,
		}
		cc.MinHtlcIn = cfg.Bitcoin.MinHTLCIn
		cc.FeeEstimator = chainfee.NewStaticEstimator(
			defaultBitcoinStaticFeePerKW,
			defaultBitcoinStaticMinRelayFeeRate,
		)
	case chainreg.LitecoinChain:
		cc.RoutingPolicy = htlcswitch.ForwardingPolicy{
			MinHTLCOut:    cfg.Litecoin.MinHTLCOut,
			BaseFee:       cfg.Litecoin.BaseFee,
			FeeRate:       cfg.Litecoin.FeeRate,
			TimeLockDelta: cfg.Litecoin.TimeLockDelta,
		}
		cc.MinHtlcIn = cfg.Litecoin.MinHTLCIn
		cc.FeeEstimator = chainfee.NewStaticEstimator(
			defaultLitecoinStaticFeePerKW, 0,
		)
	default:
		return nil, fmt.Errorf("default routing policy for chain %v is "+
			"unknown", cfg.PrimaryChain())
	}

	walletConfig := &btcwallet.Config{
		PrivatePass:    cfg.PrivateWalletPw,
		PublicPass:     cfg.PublicWalletPw,
		Birthday:       cfg.Birthday,
		RecoveryWindow: cfg.RecoveryWindow,
		DataDir:        homeChainConfig.ChainDir,
		NetParams:      cfg.ActiveNetParams.Params,
		CoinType:       cfg.ActiveNetParams.CoinType,
		Wallet:         cfg.Wallet,
	}

	var err error

	heightHintCacheConfig := chainntnfs.CacheConfig{
		QueryDisable: cfg.HeightHintCacheQueryDisable,
	}
	if cfg.HeightHintCacheQueryDisable {
		ltndLog.Infof("Height Hint Cache Queries disabled")
	}

	// Initialize the height hint cache within the chain directory.
	hintCache, err := chainntnfs.NewHeightHintCache(
		heightHintCacheConfig, cfg.LocalChanDB,
	)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize height hint "+
			"cache: %v", err)
	}

	// If spv mode is active, then we'll be using a distinct set of
	// chainControl interfaces that interface directly with the p2p network
	// of the selected chain.
	switch homeChainConfig.Node {
	case "neutrino":
		// We'll create ChainNotifier and FilteredChainView instances,
		// along with the wallet's ChainSource, which are all backed by
		// the neutrino light client.
		cc.ChainNotifier = neutrinonotify.New(
			cfg.NeutrinoCS, hintCache, hintCache,
		)
		cc.ChainView, err = chainview.NewCfFilteredChainView(cfg.NeutrinoCS)
		if err != nil {
			return nil, err
		}

		// Map the deprecated neutrino feeurl flag to the general fee
		// url.
		if cfg.NeutrinoMode.FeeURL != "" {
			if cfg.FeeURL != "" {
				return nil, errors.New("feeurl and " +
					"neutrino.feeurl are mutually exclusive")
			}

			cfg.FeeURL = cfg.NeutrinoMode.FeeURL
		}

		walletConfig.ChainSource = chain.NewNeutrinoClient(
			cfg.ActiveNetParams.Params, cfg.NeutrinoCS,
		)

	case "bitcoind", "litecoind":
		var bitcoindMode *lncfg.Bitcoind
		switch {
		case cfg.Bitcoin.Active:
			bitcoindMode = cfg.BitcoindMode
		case cfg.Litecoin.Active:
			bitcoindMode = cfg.LitecoindMode
		}
		// Otherwise, we'll be speaking directly via RPC and ZMQ to a
		// bitcoind node. If the specified host for the btcd/ltcd RPC
		// server already has a port specified, then we use that
		// directly. Otherwise, we assume the default port according to
		// the selected chain parameters.
		var bitcoindHost string
		if strings.Contains(bitcoindMode.RPCHost, ":") {
			bitcoindHost = bitcoindMode.RPCHost
		} else {
			// The RPC ports specified in chainparams.go assume
			// btcd, which picks a different port so that btcwallet
			// can use the same RPC port as bitcoind. We convert
			// this back to the btcwallet/bitcoind port.
			rpcPort, err := strconv.Atoi(cfg.ActiveNetParams.RPCPort)
			if err != nil {
				return nil, err
			}
			rpcPort -= 2
			bitcoindHost = fmt.Sprintf("%v:%d",
				bitcoindMode.RPCHost, rpcPort)
			if (cfg.Bitcoin.Active && cfg.Bitcoin.RegTest) ||
				(cfg.Litecoin.Active && cfg.Litecoin.RegTest) {
				conn, err := net.Dial("tcp", bitcoindHost)
				if err != nil || conn == nil {
					if cfg.Bitcoin.Active && cfg.Bitcoin.RegTest {
						rpcPort = 18443
					} else if cfg.Litecoin.Active && cfg.Litecoin.RegTest {
						rpcPort = 19443
					}
					bitcoindHost = fmt.Sprintf("%v:%d",
						bitcoindMode.RPCHost,
						rpcPort)
				} else {
					conn.Close()
				}
			}
		}

		// Establish the connection to bitcoind and create the clients
		// required for our relevant subsystems.
		bitcoindConn, err := chain.NewBitcoindConn(
			cfg.ActiveNetParams.Params, bitcoindHost,
			bitcoindMode.RPCUser, bitcoindMode.RPCPass,
			bitcoindMode.ZMQPubRawBlock, bitcoindMode.ZMQPubRawTx,
			5*time.Second,
		)
		if err != nil {
			return nil, err
		}

		if err := bitcoindConn.Start(); err != nil {
			return nil, fmt.Errorf("unable to connect to bitcoind: "+
				"%v", err)
		}

		cc.ChainNotifier = bitcoindnotify.New(
			bitcoindConn, cfg.ActiveNetParams.Params, hintCache, hintCache,
		)
		cc.ChainView = chainview.NewBitcoindFilteredChainView(bitcoindConn)
		walletConfig.ChainSource = bitcoindConn.NewBitcoindClient()

		// If we're not in regtest mode, then we'll attempt to use a
		// proper fee estimator for testnet.
		rpcConfig := &rpcclient.ConnConfig{
			Host:                 bitcoindHost,
			User:                 bitcoindMode.RPCUser,
			Pass:                 bitcoindMode.RPCPass,
			DisableConnectOnNew:  true,
			DisableAutoReconnect: false,
			DisableTLS:           true,
			HTTPPostMode:         true,
		}
		if cfg.Bitcoin.Active && !cfg.Bitcoin.RegTest {
			ltndLog.Infof("Initializing bitcoind backed fee estimator in "+
				"%s mode", bitcoindMode.EstimateMode)

			// Finally, we'll re-initialize the fee estimator, as
			// if we're using bitcoind as a backend, then we can
			// use live fee estimates, rather than a statically
			// coded value.
			fallBackFeeRate := chainfee.SatPerKVByte(25 * 1000)
			cc.FeeEstimator, err = chainfee.NewBitcoindEstimator(
				*rpcConfig, bitcoindMode.EstimateMode,
				fallBackFeeRate.FeePerKWeight(),
			)
			if err != nil {
				return nil, err
			}
		} else if cfg.Litecoin.Active && !cfg.Litecoin.RegTest {
			ltndLog.Infof("Initializing litecoind backed fee estimator in "+
				"%s mode", bitcoindMode.EstimateMode)

			// Finally, we'll re-initialize the fee estimator, as
			// if we're using litecoind as a backend, then we can
			// use live fee estimates, rather than a statically
			// coded value.
			fallBackFeeRate := chainfee.SatPerKVByte(25 * 1000)
			cc.FeeEstimator, err = chainfee.NewBitcoindEstimator(
				*rpcConfig, bitcoindMode.EstimateMode,
				fallBackFeeRate.FeePerKWeight(),
			)
			if err != nil {
				return nil, err
			}
		}
	case "btcd", "ltcd":
		// Otherwise, we'll be speaking directly via RPC to a node.
		//
		// So first we'll load btcd/ltcd's TLS cert for the RPC
		// connection. If a raw cert was specified in the config, then
		// we'll set that directly. Otherwise, we attempt to read the
		// cert from the path specified in the config.
		var btcdMode *lncfg.Btcd
		switch {
		case cfg.Bitcoin.Active:
			btcdMode = cfg.BtcdMode
		case cfg.Litecoin.Active:
			btcdMode = cfg.LtcdMode
		}
		var rpcCert []byte
		if btcdMode.RawRPCCert != "" {
			rpcCert, err = hex.DecodeString(btcdMode.RawRPCCert)
			if err != nil {
				return nil, err
			}
		} else {
			certFile, err := os.Open(btcdMode.RPCCert)
			if err != nil {
				return nil, err
			}
			rpcCert, err = ioutil.ReadAll(certFile)
			if err != nil {
				return nil, err
			}
			if err := certFile.Close(); err != nil {
				return nil, err
			}
		}

		// If the specified host for the btcd/ltcd RPC server already
		// has a port specified, then we use that directly. Otherwise,
		// we assume the default port according to the selected chain
		// parameters.
		var btcdHost string
		if strings.Contains(btcdMode.RPCHost, ":") {
			btcdHost = btcdMode.RPCHost
		} else {
			btcdHost = fmt.Sprintf("%v:%v", btcdMode.RPCHost,
				cfg.ActiveNetParams.RPCPort)
		}

		btcdUser := btcdMode.RPCUser
		btcdPass := btcdMode.RPCPass
		rpcConfig := &rpcclient.ConnConfig{
			Host:                 btcdHost,
			Endpoint:             "ws",
			User:                 btcdUser,
			Pass:                 btcdPass,
			Certificates:         rpcCert,
			DisableTLS:           false,
			DisableConnectOnNew:  true,
			DisableAutoReconnect: false,
		}
		cc.ChainNotifier, err = btcdnotify.New(
			rpcConfig, cfg.ActiveNetParams.Params, hintCache, hintCache,
		)
		if err != nil {
			return nil, err
		}

		// Finally, we'll create an instance of the default chain view to be
		// used within the routing layer.
		cc.ChainView, err = chainview.NewBtcdFilteredChainView(*rpcConfig)
		if err != nil {
			srvrLog.Errorf("unable to create chain view: %v", err)
			return nil, err
		}

		// Create a special websockets rpc client for btcd which will be used
		// by the wallet for notifications, calls, etc.
		chainRPC, err := chain.NewRPCClient(cfg.ActiveNetParams.Params, btcdHost,
			btcdUser, btcdPass, rpcCert, false, 20)
		if err != nil {
			return nil, err
		}

		walletConfig.ChainSource = chainRPC

		// If we're not in simnet or regtest mode, then we'll attempt
		// to use a proper fee estimator for testnet.
		if !cfg.Bitcoin.SimNet && !cfg.Litecoin.SimNet &&
			!cfg.Bitcoin.RegTest && !cfg.Litecoin.RegTest {

			ltndLog.Infof("Initializing btcd backed fee estimator")

			// Finally, we'll re-initialize the fee estimator, as
			// if we're using btcd as a backend, then we can use
			// live fee estimates, rather than a statically coded
			// value.
			fallBackFeeRate := chainfee.SatPerKVByte(25 * 1000)
			cc.FeeEstimator, err = chainfee.NewBtcdEstimator(
				*rpcConfig, fallBackFeeRate.FeePerKWeight(),
			)
			if err != nil {
				return nil, err
			}
		}
	default:
		return nil, fmt.Errorf("unknown node type: %s",
			homeChainConfig.Node)
	}

	// Override default fee estimator if an external service is specified.
	if cfg.FeeURL != "" {
		// Do not cache fees on regtest to make it easier to execute
		// manual or automated test cases.
		cacheFees := !cfg.Bitcoin.RegTest

		ltndLog.Infof("Using external fee estimator %v: cached=%v",
			cfg.FeeURL, cacheFees)

		cc.FeeEstimator = chainfee.NewWebAPIEstimator(
			chainfee.SparseConfFeeSource{
				URL: cfg.FeeURL,
			},
			!cacheFees,
		)
	}

	// Start fee estimator.
	if err := cc.FeeEstimator.Start(); err != nil {
		return nil, err
	}

	wc, err := btcwallet.New(*walletConfig)
	if err != nil {
		fmt.Printf("unable to create wallet controller: %v\n", err)
		return nil, err
	}

	cc.MsgSigner = wc
	cc.Signer = wc
	cc.ChainIO = wc
	cc.Wc = wc

	// Select the default channel constraints for the primary chain.
	channelConstraints := DefaultBtcChannelConstraints
	if cfg.PrimaryChain() == chainreg.LitecoinChain {
		channelConstraints = DefaultLtcChannelConstraints
	}

	keyRing := keychain.NewBtcWalletKeyRing(
		wc.InternalWallet(), cfg.ActiveNetParams.CoinType,
	)
	cc.KeyRing = keyRing

	// Create, and start the lnwallet, which handles the core payment
	// channel logic, and exposes control via proxy state machines.
	walletCfg := lnwallet.Config{
		Database:           cfg.RemoteChanDB,
		Notifier:           cc.ChainNotifier,
		WalletController:   wc,
		Signer:             cc.Signer,
		FeeEstimator:       cc.FeeEstimator,
		SecretKeyRing:      keyRing,
		ChainIO:            cc.ChainIO,
		DefaultConstraints: channelConstraints,
		NetParams:          *cfg.ActiveNetParams.Params,
	}
	lnWallet, err := lnwallet.NewLightningWallet(walletCfg)
	if err != nil {
		fmt.Printf("unable to create wallet: %v\n", err)
		return nil, err
	}
	if err := lnWallet.Startup(); err != nil {
		fmt.Printf("unable to start wallet: %v\n", err)
		return nil, err
	}

	ltndLog.Info("LightningWallet opened")

	cc.Wallet = lnWallet

	return cc, nil
}

var (
	// BitcoinTestnetGenesis is the genesis hash of Bitcoin's testnet
	// chain.
	BitcoinTestnetGenesis = chainhash.Hash([chainhash.HashSize]byte{
		0x43, 0x49, 0x7f, 0xd7, 0xf8, 0x26, 0x95, 0x71,
		0x08, 0xf4, 0xa3, 0x0f, 0xd9, 0xce, 0xc3, 0xae,
		0xba, 0x79, 0x97, 0x20, 0x84, 0xe9, 0x0e, 0xad,
		0x01, 0xea, 0x33, 0x09, 0x00, 0x00, 0x00, 0x00,
	})

	// BitcoinMainnetGenesis is the genesis hash of Bitcoin's main chain.
	BitcoinMainnetGenesis = chainhash.Hash([chainhash.HashSize]byte{
		0x6f, 0xe2, 0x8c, 0x0a, 0xb6, 0xf1, 0xb3, 0x72,
		0xc1, 0xa6, 0xa2, 0x46, 0xae, 0x63, 0xf7, 0x4f,
		0x93, 0x1e, 0x83, 0x65, 0xe1, 0x5a, 0x08, 0x9c,
		0x68, 0xd6, 0x19, 0x00, 0x00, 0x00, 0x00, 0x00,
	})

	// LitecoinTestnetGenesis is the genesis hash of Litecoin's testnet4
	// chain.
	LitecoinTestnetGenesis = chainhash.Hash([chainhash.HashSize]byte{
		0xa0, 0x29, 0x3e, 0x4e, 0xeb, 0x3d, 0xa6, 0xe6,
		0xf5, 0x6f, 0x81, 0xed, 0x59, 0x5f, 0x57, 0x88,
		0x0d, 0x1a, 0x21, 0x56, 0x9e, 0x13, 0xee, 0xfd,
		0xd9, 0x51, 0x28, 0x4b, 0x5a, 0x62, 0x66, 0x49,
	})

	// LitecoinMainnetGenesis is the genesis hash of Litecoin's main chain.
	LitecoinMainnetGenesis = chainhash.Hash([chainhash.HashSize]byte{
		0xe2, 0xbf, 0x04, 0x7e, 0x7e, 0x5a, 0x19, 0x1a,
		0xa4, 0xef, 0x34, 0xd3, 0x14, 0x97, 0x9d, 0xc9,
		0x98, 0x6e, 0x0f, 0x19, 0x25, 0x1e, 0xda, 0xba,
		0x59, 0x40, 0xfd, 0x1f, 0xe3, 0x65, 0xa7, 0x12,
	})

	// chainMap is a simple index that maps a chain's genesis hash to the
	// ChainCode enum for that chain.
	chainMap = map[chainhash.Hash]chainreg.ChainCode{
		BitcoinTestnetGenesis:  chainreg.BitcoinChain,
		LitecoinTestnetGenesis: chainreg.LitecoinChain,

		BitcoinMainnetGenesis:  chainreg.BitcoinChain,
		LitecoinMainnetGenesis: chainreg.LitecoinChain,
	}

	// ChainDNSSeeds is a map of a chain's hash to the set of DNS seeds
	// that will be use to bootstrap peers upon first startup.
	//
	// The first item in the array is the primary host we'll use to attempt
	// the SRV lookup we require. If we're unable to receive a response
	// over UDP, then we'll fall back to manual TCP resolution. The second
	// item in the array is a special A record that we'll query in order to
	// receive the IP address of the current authoritative DNS server for
	// the network seed.
	//
	// TODO(roasbeef): extend and collapse these and chainparams.go into
	// struct like chaincfg.Params
	ChainDNSSeeds = map[chainhash.Hash][][2]string{
		BitcoinMainnetGenesis: {
			{
				"nodes.lightning.directory",
				"soa.nodes.lightning.directory",
			},
			{
				"lseed.bitcoinstats.com",
			},
		},

		BitcoinTestnetGenesis: {
			{
				"test.nodes.lightning.directory",
				"soa.nodes.lightning.directory",
			},
		},

		LitecoinMainnetGenesis: {
			{
				"ltc.nodes.lightning.directory",
				"soa.nodes.lightning.directory",
			},
		},
	}
)

// ChainRegistry keeps track of the current chains
type ChainRegistry struct {
	sync.RWMutex

	activeChains map[chainreg.ChainCode]*ChainControl
	netParams    map[chainreg.ChainCode]*chainreg.BitcoinNetParams

	primaryChain chainreg.ChainCode
}

// NewChainRegistry creates a new ChainRegistry.
func NewChainRegistry() *ChainRegistry {
	return &ChainRegistry{
		activeChains: make(map[chainreg.ChainCode]*ChainControl),
		netParams:    make(map[chainreg.ChainCode]*chainreg.BitcoinNetParams),
	}
}

// RegisterChain assigns an active ChainControl instance to a target chain
// identified by its ChainCode.
func (c *ChainRegistry) RegisterChain(newChain chainreg.ChainCode,
	cc *ChainControl) {
	
	c.Lock()
	c.activeChains[newChain] = cc
	c.Unlock()
}

// LookupChain attempts to lookup an active ChainControl instance for the
// target chain.
func (c *ChainRegistry) LookupChain(targetChain chainreg.ChainCode) (
	*ChainControl, bool) {
	
	c.RLock()
	cc, ok := c.activeChains[targetChain]
	c.RUnlock()
	return cc, ok
}

// LookupChainByHash attempts to look up an active ChainControl which
// corresponds to the passed genesis hash.
func (c *ChainRegistry) LookupChainByHash(chainHash chainhash.Hash) (*ChainControl, bool) {
	c.RLock()
	defer c.RUnlock()

	targetChain, ok := chainMap[chainHash]
	if !ok {
		return nil, ok
	}

	cc, ok := c.activeChains[targetChain]
	return cc, ok
}

// RegisterPrimaryChain sets a target chain as the "home chain" for lnd.
func (c *ChainRegistry) RegisterPrimaryChain(cc chainreg.ChainCode) {
	c.Lock()
	defer c.Unlock()

	c.primaryChain = cc
}

// PrimaryChain returns the primary chain for this running lnd instance. The
// primary chain is considered the "home base" while the other registered
// chains are treated as secondary chains.
func (c *ChainRegistry) PrimaryChain() chainreg.ChainCode {
	c.RLock()
	defer c.RUnlock()

	return c.primaryChain
}

// ActiveChains returns a slice containing the active chains.
func (c *ChainRegistry) ActiveChains() []chainreg.ChainCode {
	c.RLock()
	defer c.RUnlock()

	chains := make([]chainreg.ChainCode, 0, len(c.activeChains))
	for activeChain := range c.activeChains {
		chains = append(chains, activeChain)
	}

	return chains
}

// NumActiveChains returns the total number of active chains.
func (c *ChainRegistry) NumActiveChains() uint32 {
	c.RLock()
	defer c.RUnlock()

	return uint32(len(c.activeChains))
}

// InitNeutrinoBackend inits a new instance of the neutrino light client
// backend given a target chain directory to store the chain state.
func InitNeutrinoBackend(cfg *lncfg.Neutrino, tornet tor.Net,
	syncFreelist bool, chainDir string,
	netParams chainreg.BitcoinNetParams) (*neutrino.ChainService,
	func(), error) {

	// First we'll open the database file for neutrino, creating the
	// database if needed. We append the normalized network name here to
	// match the behavior of btcwallet.
	dbPath := filepath.Join(
		chainDir,
		lncfg.NormalizeNetwork(netParams.Name),
	)

	// Ensure that the neutrino db path exists.
	if err := os.MkdirAll(dbPath, 0700); err != nil {
		return nil, nil, err
	}

	dbName := filepath.Join(dbPath, "neutrino.db")
	db, err := walletdb.Create("bdb", dbName, !syncFreelist)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to create neutrino "+
			"database: %v", err)
	}

	headerStateAssertion, err := parseHeaderStateAssertion(
		cfg.AssertFilterHeader,
	)
	if err != nil {
		db.Close()
		return nil, nil, err
	}

	// With the database open, we can now create an instance of the
	// neutrino light client. We pass in relevant configuration parameters
	// required.
	config := neutrino.Config{
		DataDir:      dbPath,
		Database:     db,
		ChainParams:  *netParams.Params,
		AddPeers:     cfg.AddPeers,
		ConnectPeers: cfg.ConnectPeers,
		Dialer: func(addr net.Addr) (net.Conn, error) {
			return tornet.Dial(
				addr.Network(), addr.String(),
				cfg.ConnectionTimeout,
			)
		},
		NameResolver: func(host string) ([]net.IP, error) {
			addrs, err := tornet.LookupHost(host)
			if err != nil {
				return nil, err
			}

			ips := make([]net.IP, 0, len(addrs))
			for _, strIP := range addrs {
				ip := net.ParseIP(strIP)
				if ip == nil {
					continue
				}

				ips = append(ips, ip)
			}

			return ips, nil
		},
		AssertFilterHeader: headerStateAssertion,
	}

	neutrino.MaxPeers = 8
	neutrino.BanDuration = time.Hour * 48
	neutrino.UserAgentName = cfg.NeutrinoMode.UserAgentName
	neutrino.UserAgentVersion = cfg.NeutrinoMode.UserAgentVersion

	neutrinoCS, err := neutrino.NewChainService(config)
	if err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("unable to create neutrino light "+
			"client: %v", err)
	}

	if err := neutrinoCS.Start(); err != nil {
		db.Close()
		return nil, nil, err
	}

	cleanUp := func() {
		neutrinoCS.Stop()
		db.Close()
	}

	return neutrinoCS, cleanUp, nil
}

// parseHeaderStateAssertion parses the user-specified neutrino header state
// into a headerfs.FilterHeader.
func parseHeaderStateAssertion(state string) (*headerfs.FilterHeader, error) {
	if len(state) == 0 {
		return nil, nil
	}

	split := strings.Split(state, ":")
	if len(split) != 2 {
		return nil, fmt.Errorf("header state assertion %v in "+
			"unexpected format, expected format height:hash", state)
	}

	height, err := strconv.ParseUint(split[0], 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid filter header height: %v", err)
	}

	hash, err := chainhash.NewHashFromStr(split[1])
	if err != nil {
		return nil, fmt.Errorf("invalid filter header hash: %v", err)
	}

	return &headerfs.FilterHeader{
		Height:     uint32(height),
		FilterHash: *hash,
	}, nil
}
