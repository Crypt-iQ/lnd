# using zero-conf

## startup

- must specify `--protocol.option-scid-alias --protocol.zero-conf`
- should specify an rpc channel acceptor so auto-deny is off

## opening channel

### initiator

- anchors channeltype must be on
- zero_conf flag either for openchannel or rpc must be true

### responder

- rpc channel acceptor must be registered to be able to accept
- the channeltype isnt surfaced, so you must know the opener is trying zeroconf
- to accept, set ZeroConf = true in acceptor response, false otherwise

## listchannels

this rpc will give you the confirmed scid in the ChanId field if this is not zero-conf
for zero-conf, this will give the first alias scid and after 6 confs, the confirmed
scid will be there instead.
this rpc also returns the list of aliases for the channel as `[]uint64`

### other rpcs

the following rpc structs may use an alias instead of the confirmed scid and vice versa:
- `InvoiceHtlc`
- `ChannelFeeReport`
- `ChannelUpdate`
- `Channel`
- `ChannelCloseSummary`
- `HopHint`

calling code should compare the ChanId in these structs with an alias in listchannels.
it is easy to determine if these are an alias as the block height will be >= 16_000_000