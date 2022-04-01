package itest

import (
	"context"
	"testing"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/wire"
	"github.com/go-errors/errors"
	"github.com/lightningnetwork/lnd/chainreg"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnrpc/routerrpc"
	"github.com/lightningnetwork/lnd/lntest"
	"github.com/lightningnetwork/lnd/lntest/wait"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/stretchr/testify/require"
)

// testZeroConfChannelOpen tests that opening a zero-conf channel works and
// sending payments also works.
func testZeroConfChannelOpen(net *lntest.NetworkHarness, t *harnessTest) {
	// Since option-scid-alias is opt-in, the provided harness nodes will
	// not have the feature bit set. Also need to set anchors as those are
	// default-off in itests.
	scidAliasArgs := []string{
		"--protocol.option-scid-alias",
		"--protocol.anchors",
	}

	carol := net.NewNode(t.t, "Carol", scidAliasArgs)
	defer shutdownAndAssert(net, t, carol)

	// We'll open a regular public channel between Bob and Carol here.
	net.EnsureConnected(t.t, net.Bob, carol)

	chanAmt := btcutil.Amount(1_000_000)

	fundingPoint := openChannelAndAssert(
		t, net, net.Bob, carol,
		lntest.OpenChannelParams{
			Amt: chanAmt,
		},
	)

	// Wait for both Bob and Carol to view the channel as active.
	ctxb := context.Background()
	err := net.Bob.WaitForNetworkChannelOpen(fundingPoint)
	require.NoError(t.t, err, "bob didn't report channel")
	err = carol.WaitForNetworkChannelOpen(fundingPoint)
	require.NoError(t.t, err, "carol didn't report channel")

	// Spin-up Dave so Carol can open a zero-conf channel to him.
	dave := net.NewNode(t.t, "Dave", scidAliasArgs)
	defer shutdownAndAssert(net, t, dave)

	// We'll give Carol some coins in order to fund the channel.
	net.SendCoins(t.t, btcutil.SatoshiPerBitcoin, carol)

	// Ensure that both Carol and Dave are connected.
	net.EnsureConnected(t.t, carol, dave)

	// Open a private zero-conf anchors channel of 1M satoshis.
	params := lntest.OpenChannelParams{
		Amt:            chanAmt,
		Private:        true,
		CommitmentType: lnrpc.CommitmentType_ANCHORS,
		ZeroConf:       true,
	}
	chanOpenUpdate := openChannelStream(t, net, carol, dave, params)

	// We should receive the OpenStatusUpdate_ChanOpen update without
	// having to mine any blocks.
	fundingPoint2, err := net.WaitForChannelOpen(chanOpenUpdate)
	require.NoError(t.t, err, "error while waiting for channel open")

	err = carol.WaitForNetworkChannelOpen(fundingPoint2)
	require.NoError(t.t, err, "carol didn't report channel")
	err = dave.WaitForNetworkChannelOpen(fundingPoint2)
	require.NoError(t.t, err, "dave didn't report channel")

	// Attempt to send a 10K satoshi payment from Carol to Dave.
	daveInvoiceParams := &lnrpc.Invoice{
		Value:   int64(10_000),
		Private: true,
	}
	daveInvoiceResp, err := dave.AddInvoice(
		ctxb, daveInvoiceParams,
	)
	require.NoError(t.t, err, "unable to add invoice")
	_ = sendAndAssertSuccess(
		t, carol, &routerrpc.SendPaymentRequest{
			PaymentRequest: daveInvoiceResp.PaymentRequest,
			TimeoutSeconds: 60,
			FeeLimitMsat:   noFeeLimitMsat,
		},
	)

	// Now attempt to send a multi-hop payment from Bob to Dave. This tests
	// that Dave issues an invoice with an alias SCID that Carol knows and
	// uses to forward to Dave.
	daveInvoiceResp2, err := dave.AddInvoice(
		ctxb, daveInvoiceParams,
	)
	require.NoError(t.t, err, "unable to add invoice")
	_ = sendAndAssertSuccess(
		t, net.Bob, &routerrpc.SendPaymentRequest{
			PaymentRequest: daveInvoiceResp2.PaymentRequest,
			TimeoutSeconds: 60,
			FeeLimitMsat:   noFeeLimitMsat,
		},
	)

	// We'll now confirm the zero-conf channel between Carol and Dave and
	// assert that sending is still possible.
	block := mineBlocks(t, net, 6, 1)[0]

	fundingTxID, err := lnrpc.GetChanPointFundingTxid(fundingPoint2)
	require.NoError(t.t, err, "unable to get txid")

	assertTxInBlock(t, block, fundingTxID)

	descReq := &lnrpc.ChannelGraphRequest{
		IncludeUnannounced: true,
	}

	// Wait until Dave's ZeroConf channel with the correct ChannelID is
	// found locally in his graph.
	err = waitForZeroConfInGraph(ctxb, dave, descReq)
	require.NoError(t.t, err, "expected to not receive error")

	daveInvoiceResp3, err := dave.AddInvoice(
		ctxb, daveInvoiceParams,
	)
	require.NoError(t.t, err, "unable to add invoice")
	_ = sendAndAssertSuccess(
		t, net.Bob, &routerrpc.SendPaymentRequest{
			PaymentRequest: daveInvoiceResp3.PaymentRequest,
			TimeoutSeconds: 60,
			FeeLimitMsat:   noFeeLimitMsat,
		},
	)

	// Eve will now initiate a zero-conf channel with Carol. This tests
	// that the ChannelUpdates sent are correct since they will be
	// referring to different alias SCIDs.
	eve := net.NewNode(t.t, "Eve", scidAliasArgs)
	defer shutdownAndAssert(net, t, eve)

	net.EnsureConnected(t.t, eve, carol)

	// Give Eve some coins to fund the channel.
	net.SendCoins(t.t, btcutil.SatoshiPerBitcoin, eve)

	// We'll open a public zero-conf anchors channel of 1M satoshis.
	params.Private = false
	chanOpenUpdate2 := openChannelStream(t, net, eve, carol, params)

	// Wait to receive the OpenStatusUpdate_ChanOpen update.
	fundingPoint3, err := net.WaitForChannelOpen(chanOpenUpdate2)
	require.NoError(t.t, err, "error while waiting for channel open")

	err = eve.WaitForNetworkChannelOpen(fundingPoint3)
	require.NoError(t.t, err, "eve didn't report channel")
	err = carol.WaitForNetworkChannelOpen(fundingPoint3)
	require.NoError(t.t, err, "carol didn't report channel")

	// Attempt to send a 20K satoshi payment from Eve to Dave.
	daveInvoiceParams.Value = int64(20_000)
	daveInvoiceResp4, err := dave.AddInvoice(
		ctxb, daveInvoiceParams,
	)
	require.NoError(t.t, err, "unable to add invoice")
	_ = sendAndAssertSuccess(
		t, eve, &routerrpc.SendPaymentRequest{
			PaymentRequest: daveInvoiceResp4.PaymentRequest,
			TimeoutSeconds: 60,
			FeeLimitMsat:   noFeeLimitMsat,
		},
	)

	// We'll confirm the zero-conf channel between Eve and Carol and assert
	// that sending is still possible.
	block = mineBlocks(t, net, 6, 1)[0]

	fundingTxID, err = lnrpc.GetChanPointFundingTxid(fundingPoint3)
	require.NoError(t.t, err, "unable to get txid")

	assertTxInBlock(t, block, fundingTxID)

	// Wait until Eve's ZeroConf channel is found locally in her graph.
	err = waitForZeroConfInGraph(ctxb, eve, descReq)
	require.NoError(t.t, err, "expected to not receive error")

	// Attempt to send a 6K satoshi payment from Dave to Eve.
	eveInvoiceParams := &lnrpc.Invoice{
		Value:   int64(6_000),
		Private: true,
	}
	eveInvoiceResp, err := eve.AddInvoice(ctxb, eveInvoiceParams)
	require.NoError(t.t, err, "unable to add invoice")
	_ = sendAndAssertSuccess(
		t, dave, &routerrpc.SendPaymentRequest{
			PaymentRequest: eveInvoiceResp.PaymentRequest,
			TimeoutSeconds: 60,
			FeeLimitMsat:   noFeeLimitMsat,
		},
	)
}

// testPrivateOptionScidAlias checks that opening a private option_scid_alias
// channel works properly.
func testPrivateOptionScidAlias(net *lntest.NetworkHarness, t *harnessTest) {
	ctxb := context.Background()

	// Option-scid-alias is opt-in, as is anchors.
	scidAliasArgs := []string{
		"--protocol.option-scid-alias",
		"--protocol.anchors",
	}

	carol := net.NewNode(t.t, "Carol", scidAliasArgs)
	defer shutdownAndAssert(net, t, carol)

	dave := net.NewNode(t.t, "Dave", scidAliasArgs)
	defer shutdownAndAssert(net, t, dave)

	// Ensure Carol, Dave are connected.
	net.EnsureConnected(t.t, carol, dave)

	// Give Carol some coins so she can open the channel.
	net.SendCoins(t.t, btcutil.SatoshiPerBitcoin, carol)

	chanAmt := btcutil.Amount(1_000_000)

	params := lntest.OpenChannelParams{
		Amt:            chanAmt,
		Private:        true,
		CommitmentType: lnrpc.CommitmentType_ANCHORS,
		ScidAlias:      true,
	}
	fundingPoint := openChannelAndAssert(t, net, carol, dave, params)

	err := carol.WaitForNetworkChannelOpen(fundingPoint)
	require.NoError(t.t, err, "carol didn't report channel")
	err = dave.WaitForNetworkChannelOpen(fundingPoint)
	require.NoError(t.t, err, "dave didn't report channel")

	// Assert that a payment from Carol to Dave works as expected.
	daveInvoiceParams := &lnrpc.Invoice{
		Value:   int64(10_000),
		Private: true,
	}
	daveInvoiceResp, err := dave.AddInvoice(ctxb, daveInvoiceParams)
	require.NoError(t.t, err)
	_ = sendAndAssertSuccess(
		t, carol, &routerrpc.SendPaymentRequest{
			PaymentRequest: daveInvoiceResp.PaymentRequest,
			TimeoutSeconds: 60,
			FeeLimitMsat:   noFeeLimitMsat,
		},
	)

	// We'll now open a regular public channel between Bob and Carol and
	// assert that Bob can pay Dave. We'll also assert that the invoice
	// Dave issues has the startingAlias as a hop hint.
	net.EnsureConnected(t.t, net.Bob, carol)

	fundingPoint2 := openChannelAndAssert(
		t, net, net.Bob, carol,
		lntest.OpenChannelParams{
			Amt: chanAmt,
		},
	)

	err = net.Bob.WaitForNetworkChannelOpen(fundingPoint2)
	require.NoError(t.t, err)
	err = carol.WaitForNetworkChannelOpen(fundingPoint2)
	require.NoError(t.t, err)

	// Wait until Dave receives the Bob<->Carol channel.
	err = dave.WaitForNetworkChannelOpen(fundingPoint2)
	require.NoError(t.t, err)

	daveInvoiceResp2, err := dave.AddInvoice(ctxb, daveInvoiceParams)
	require.NoError(t.t, err)
	davePayReq := &lnrpc.PayReqString{
		PayReq: daveInvoiceResp2.PaymentRequest,
	}
	decodedReq, err := dave.DecodePayReq(ctxb, davePayReq)
	require.NoError(t.t, err)
	require.Equal(t.t, 1, len(decodedReq.RouteHints))
	require.Equal(t.t, 1, len(decodedReq.RouteHints[0].HopHints))

	startingAlias := lnwire.ShortChannelID{
		BlockHeight: 10000,
		TxIndex:     0,
		TxPosition:  0,
	}

	daveHopHint := decodedReq.RouteHints[0].HopHints[0].ChanId
	require.Equal(t.t, startingAlias.ToUint64(), daveHopHint)
}

// waitForZeroConfInGraph waits for the zero-conf channel to be visible in the
// graph after confirmation.
func waitForZeroConfInGraph(ctxb context.Context, n *lntest.HarnessNode,
	req *lnrpc.ChannelGraphRequest) error {

	return wait.NoError(func() error {
		ctxt, _ := context.WithTimeout(ctxb, defaultTimeout)
		graph, err := n.DescribeGraph(ctxt, req)
		if err != nil {
			return err
		}

		var found bool

		for _, e := range graph.Edges {
			scid := lnwire.NewShortChanIDFromInt(e.ChannelId)
			if scid.BlockHeight >= 10000 {
				continue
			}

			// Both edge policies must exist.
			if e.Node1Policy == nil || e.Node2Policy == nil {
				continue
			}

			if e.Node1Pub == n.PubKeyStr ||
				e.Node2Pub == n.PubKeyStr {
				found = true
			}
		}

		if found {
			return nil
		}

		return errors.New("failed to find zero-conf channel in graph")
	}, defaultTimeout)
}

// testUpdateChannelPolicyPrivateAlias checks that sending a payment works
// after an option-scid-alias or zero-conf private channel updates their
// channel policy.
func testUpdateChannelPolicyPrivateAlias(net *lntest.NetworkHarness,
	t *harnessTest) {

	tests := []struct {
		name     string
		zeroConf bool
	}{
		{
			name:     "private option-scid-alias update",
			zeroConf: false,
		},
		{
			name:     "private zero-conf update",
			zeroConf: true,
		},
	}

	for _, test := range tests {
		test := test

		success := t.t.Run(test.name, func(t *testing.T) {
			ht := newHarnessTest(t, net)
			testPrivateUpdateAlias(net, ht, test.zeroConf)
		})
		if !success {
			return
		}
	}
}

func testPrivateUpdateAlias(net *lntest.NetworkHarness, t *harnessTest,
	zeroConf bool) {

	ctxb := context.Background()
	defer ctxb.Done()

	// We'll create a new node Eve that will not have option-scid-alias
	// channels.
	eve := net.NewNode(t.t, "Eve", nil)
	net.SendCoins(t.t, btcutil.SatoshiPerBitcoin, eve)
	defer shutdownAndAssert(net, t, eve)

	// Since option-scid-alias is opt-in we'll need to create the specify
	// the protocol arguments when creating a new node.
	scidAliasArgs := []string{
		"--protocol.option-scid-alias",
		"--protocol.anchors",
	}

	carol := net.NewNode(t.t, "Carol", scidAliasArgs)
	defer shutdownAndAssert(net, t, carol)

	// We'll open a regular public channel between Eve and Carol here. Eve
	// will be the one receiving the onion-encrypted ChannelUpdate.
	net.EnsureConnected(t.t, eve, carol)

	chanAmt := btcutil.Amount(1_000_000)

	fundingPoint := openChannelAndAssert(
		t, net, eve, carol,
		lntest.OpenChannelParams{
			Amt: chanAmt,
		},
	)
	defer closeChannelAndAssert(t, net, eve, fundingPoint, false)

	// Wait for both Eve and Carol to view the channel as active.
	err := eve.WaitForNetworkChannelOpen(fundingPoint)
	require.NoError(t.t, err, "eve didn't report channel")
	err = carol.WaitForNetworkChannelOpen(fundingPoint)
	require.NoError(t.t, err, "carol didn't report channel")

	// Spin-up Dave who will open the option-scid-alias or zero-conf
	// channel with Carol.
	dave := net.NewNode(t.t, "Dave", scidAliasArgs)
	defer shutdownAndAssert(net, t, dave)

	// We'll give Carol some coins in order to fund the channel.
	net.SendCoins(t.t, btcutil.SatoshiPerBitcoin, carol)

	// Ensure that Carol and Dave are connected.
	net.EnsureConnected(t.t, carol, dave)

	// Open a private channel, optionally specifying the zero-conf flag.
	params := lntest.OpenChannelParams{
		Amt:            chanAmt,
		Private:        true,
		CommitmentType: lnrpc.CommitmentType_ANCHORS,
		ZeroConf:       zeroConf,
		ScidAlias:      !zeroConf,
	}
	chanOpenUpdate := openChannelStream(t, net, carol, dave, params)

	if !zeroConf {
		_ = mineBlocks(t, net, 6, 1)
	}

	// Wait for both Carol and Dave to see the channel as open.
	fundingPoint2, err := net.WaitForChannelOpen(chanOpenUpdate)
	require.NoError(t.t, err, "error while waiting for channel open")

	err = carol.WaitForNetworkChannelOpen(fundingPoint2)
	require.NoError(t.t, err, "carol didn't report channel")
	err = dave.WaitForNetworkChannelOpen(fundingPoint2)
	require.NoError(t.t, err, "dave didn't report channel")

	// Dave will now create an invoice that Eve will use to send a payment.
	daveInvoiceParams := &lnrpc.Invoice{
		Value:   int64(10_000),
		Private: true,
	}
	daveInvoiceResp, err := dave.AddInvoice(ctxb, daveInvoiceParams)
	require.NoError(t.t, err, "unable to add invoice")

	// Carol will now update the channel edge policy for her channel with
	// Dave.
	baseFeeMSat := 33000
	timeLockDelta := uint32(chainreg.DefaultBitcoinTimeLockDelta)
	updateFeeReq := &lnrpc.PolicyUpdateRequest{
		BaseFeeMsat:   int64(baseFeeMSat),
		TimeLockDelta: timeLockDelta,
		Scope: &lnrpc.PolicyUpdateRequest_ChanPoint{
			ChanPoint: fundingPoint2,
		},
	}
	ctxt, _ := context.WithTimeout(ctxb, defaultTimeout)
	_, err = carol.UpdateChannelPolicy(ctxt, updateFeeReq)
	require.NoError(t.t, err, "unable to update chan policy")

	// Eve will pay Dave's invoice and should use the updated base fee.
	ctxt, _ = context.WithTimeout(ctxb, defaultTimeout)
	payReqs := []string{daveInvoiceResp.PaymentRequest}
	require.NoError(t.t,
		completePaymentRequests(
			eve, eve.RouterClient, payReqs, true,
		), "unable to send payment",
	)

	// Check that Eve did make the payment with two HTLCs, one failed and
	// one succeeded.
	ctxt, _ = context.WithTimeout(ctxt, defaultTimeout)
	paymentsResp, err := eve.ListPayments(
		ctxt, &lnrpc.ListPaymentsRequest{},
	)
	require.NoError(t.t, err, "failed to obtain payments for Eve")
	require.Equal(t.t, 1, len(paymentsResp.Payments), "expected 1 payment")

	htlcs := paymentsResp.Payments[0].Htlcs
	require.Equal(t.t, 2, len(htlcs), "expected to have 2 HTLCs")
	require.Equal(
		t.t, lnrpc.HTLCAttempt_FAILED, htlcs[0].Status,
		"the first HTLC attempt should fail",
	)
	require.Equal(
		t.t, lnrpc.HTLCAttempt_SUCCEEDED, htlcs[1].Status,
		"the second HTLC attempt should succeed",
	)

	eveCarolTxid, err := lnrpc.GetChanPointFundingTxid(fundingPoint)
	require.NoError(t.t, err, "unable to get txid")
	eveCarolOp := wire.OutPoint{
		Hash:  *eveCarolTxid,
		Index: fundingPoint.OutputIndex,
	}

	carolDaveTxid, err := lnrpc.GetChanPointFundingTxid(fundingPoint2)
	require.NoError(t.t, err, "unable to get txid")
	carolDaveOp := wire.OutPoint{
		Hash:  *carolDaveTxid,
		Index: fundingPoint2.OutputIndex,
	}

	// Dave should have received 20k satoshis from Carol.
	assertAmountPaid(t, "Dave(remote) [<=private] Carol(local)",
		dave, carolDaveOp, 0, 10000)

	// Carol should have sent 20k satoshis to Dave.
	assertAmountPaid(t, "Carol(local) [private=>] Dave(remote)",
		carol, carolDaveOp, 10000, 0)

	// Calcuate the amount in satoshis.
	amtExpected := int64(10000 + baseFeeMSat/1000)

	// Carol should have received 20k satoshis + fee from Eve.
	assertAmountPaid(t, "Carol(remote) <= Eve(local)",
		carol, eveCarolOp, 0, amtExpected)

	// Eve should have sent 20k satoshis + fee to Carol.
	assertAmountPaid(t, "Eve(local) => Carol(remote)",
		eve, eveCarolOp, amtExpected, 0)

	// Mine a block if zero-conf so that closeChannelAndAssert doesn't
	// throw.
	if zeroConf {
		_ = mineBlocks(t, net, 1, 1)
	}
}
