package itest

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/lightningnetwork/lnd/lnrpc/routerrpc"
	"github.com/lightningnetwork/lnd/lntest"
	"github.com/stretchr/testify/require"
)

func testTrim(net *lntest.NetworkHarness, t *harnessTest) {
	ctxb := context.Background()

	// topology:
	// a --- b --- c
	net.EnsureConnected(t.t, net.Alice, net.Bob)

	carolArgs := []string{
		"--hodl.revoke",
		"--nolisten",
		"--minbackoff=1h",
	}
	carol := net.NewNode(t.t, "Carol", carolArgs)
	defer shutdownAndAssert(net, t, carol)

	net.EnsureConnected(t.t, carol, net.Bob)

	// Create alice bob channel
	chanAmt := btcutil.Amount(1_000_000)

	chanPoint := openChannelAndAssert(
		t, net, net.Alice, net.Bob,
		lntest.OpenChannelParams{
			Amt: chanAmt,
		},
	)

	chanPoint2 := openChannelAndAssert(
		t, net, net.Bob, carol,
		lntest.OpenChannelParams{
			Amt: chanAmt,
		},
	)

	err := net.Alice.WaitForNetworkChannelOpen(chanPoint)
	require.NoError(t.t, err)
	err = net.Bob.WaitForNetworkChannelOpen(chanPoint)
	require.NoError(t.t, err)
	err = carol.WaitForNetworkChannelOpen(chanPoint)
	require.NoError(t.t, err)

	err = net.Alice.WaitForNetworkChannelOpen(chanPoint2)
	require.NoError(t.t, err)
	err = net.Bob.WaitForNetworkChannelOpen(chanPoint2)
	require.NoError(t.t, err)
	err = carol.WaitForNetworkChannelOpen(chanPoint2)
	require.NoError(t.t, err)

	// carol makes 7 invoices.
	payAmt := btcutil.Amount(10_000)
	numReqs := 7
	payReqs, _, _, err := createPayReqs(
		carol, payAmt, numReqs,
	)
	require.NoError(t.t, err)

	payFunc := func(n *lntest.HarnessNode, req string) {
		_, err := n.RouterClient.SendPaymentV2(
			ctxb,
			&routerrpc.SendPaymentRequest{
				PaymentRequest: req,
				TimeoutSeconds: 60,
				FeeLimitMsat:   noFeeLimitMsat,
				MaxParts:       1,
			},
		)
		require.NoError(t.t, err)
	}

	readDebug := func(node, target string) (bool, error) {
		// 1. find the file
		pwd, err := os.Getwd()
		if err != nil {
			return false, err
		}

		pwd += "/.logs-tranche0/"
		entries, err := os.ReadDir(pwd)
		if err != nil {
			return false, err
		}

		var nodeEntry fs.DirEntry
		for _, entry := range entries {
			if strings.Contains(entry.Name(), node) {
				nodeEntry = entry
				break
			}
		}

		pwd += nodeEntry.Name()

		fileBytes, err := os.ReadFile(pwd)
		if err != nil {
			return false, err
		}

		fileString := string(fileBytes)
		if strings.Contains(fileString, target) {
			return true, nil
		}

		return false, fmt.Errorf("missed our chance")
	}

	// alice pays the first pay req - carol won't do the dance with bob
	go payFunc(net.Alice, payReqs[0])

	// wait until carol receives a commitsig
	var firstsig bool
	for i := 0; i < 20; i++ {
		firstsig, _ = readDebug("Carol", "Received CommitSig")
		if firstsig {
			break
		}
		time.Sleep(time.Millisecond * 100)
	}
	require.True(t.t, firstsig)

	// alice sends the next one
	time.Sleep(time.Second * 2)
	go payFunc(net.Alice, payReqs[1])

	// wait until bob starts the pending ticker
	var pendingtick bool
	for i := 0; i < 20; i++ {
		pendingtick, _ = readDebug("Bob", "PendingCommitTicker resumed")
		if pendingtick {
			break
		}
		time.Sleep(time.Millisecond * 100)
	}
	require.True(t.t, pendingtick)

	// bob sends the last payment - this tests that switch-initiated payments
	// linger in the open circuits.
	time.Sleep(time.Second * 15)
	go payFunc(net.Bob, payReqs[2])

	// wait until the link goes down
	var dancefail bool
	for i := 0; i < 65; i++ {
		dancefail, _ = readDebug("Bob", "unable to complete dance")
		if dancefail {
			break
		}
		time.Sleep(time.Second)
	}
	require.True(t.t, dancefail)

	// Wait about 5 seconds for the mailbox fail to get locked on the alice
	// bob channel for the multihop payment from Alice.
	time.Sleep(time.Second * 5)

	// restart carol without the hodl.Revoke flag and get alice to attempt 2
	// more payments.
	carol.SetExtraArgs([]string{"--nolisten", "--minbackoff=1h", "--hodl.revoke"})
	if err := net.RestartNode(carol, nil); err != nil {
		t.Fatalf("Node restart failed: %v", err)
	}
	net.EnsureConnected(t.t, carol, net.Bob)

	// wait a second
	time.Sleep(time.Second * 5)

	go payFunc(net.Alice, payReqs[3])
	go payFunc(net.Alice, payReqs[4])

	var dupestone bool
	for i := 0; i < 40; i++ {
		dupestone, _ = readDebug("Bob", "duplicate keystone")
		if dupestone {
			break
		}
		time.Sleep(time.Millisecond * 100)
	}
	require.True(t.t, dupestone)

	// wait for payments to get failed back by mailbox
	time.Sleep(time.Second * 70)

	carol.SetExtraArgs([]string{"--nolisten", "--minbackoff=1h", "--hodl.revoke"})
	if err := net.RestartNode(carol, nil); err != nil {
		t.Fatalf("Node restart failed: %v", err)
	}
	net.EnsureConnected(t.t, carol, net.Bob)

	time.Sleep(time.Second * 5)

	// this allows the circuit map to recover
	go payFunc(net.Alice, payReqs[5])

	time.Sleep(time.Second * 10)

	carol.SetExtraArgs([]string{"--nolisten", "--minbackoff=1h", "--hodl.revoke"})
	if err := net.RestartNode(carol, nil); err != nil {
		t.Fatalf("Node restart failed: %v", err)
	}
	net.EnsureConnected(t.t, carol, net.Bob)

	// circuits trimmed by this point and recovery possible
	// the recovery occurs because each htlc slot up until the collision was
	// filled up so TrimOpenCircuits trimmed the conflicting switch-initiated one.
	time.Sleep(time.Second * 10)
}

func testMultiHopOnlyTrim(net *lntest.NetworkHarness, t *harnessTest) {
	ctxb := context.Background()

	net.EnsureConnected(t.t, net.Alice, net.Bob)

	carolArgs := []string{
		"--hodl.revoke",
		"--nolisten",
		"--minbackoff=1h",
	}
	carol := net.NewNode(t.t, "Carol", carolArgs)
	defer shutdownAndAssert(net, t, carol)

	net.EnsureConnected(t.t, carol, net.Bob)

	chanAmt := btcutil.Amount(1_000_000)

	chanPoint := openChannelAndAssert(
		t, net, net.Alice, net.Bob,
		lntest.OpenChannelParams{
			Amt: chanAmt,
		},
	)

	chanPoint2 := openChannelAndAssert(
		t, net, net.Bob, carol,
		lntest.OpenChannelParams{
			Amt: chanAmt,
		},
	)

	err := net.Alice.WaitForNetworkChannelOpen(chanPoint)
	require.NoError(t.t, err)
	err = net.Bob.WaitForNetworkChannelOpen(chanPoint)
	require.NoError(t.t, err)
	err = carol.WaitForNetworkChannelOpen(chanPoint)
	require.NoError(t.t, err)

	err = net.Alice.WaitForNetworkChannelOpen(chanPoint2)
	require.NoError(t.t, err)
	err = net.Bob.WaitForNetworkChannelOpen(chanPoint2)
	require.NoError(t.t, err)
	err = carol.WaitForNetworkChannelOpen(chanPoint2)
	require.NoError(t.t, err)

	// carol makes 7 invoices
	payAmt := btcutil.Amount(20_000)
	numReqs := 7
	payReqs, _, _, err := createPayReqs(
		carol, payAmt, numReqs,
	)
	require.NoError(t.t, err)

	payFunc := func(n *lntest.HarnessNode, req string) {
		_, err := n.RouterClient.SendPaymentV2(
			ctxb,
			&routerrpc.SendPaymentRequest{
				PaymentRequest: req,
				TimeoutSeconds: 60,
				FeeLimitMsat:   noFeeLimitMsat,
			},
		)
		require.NoError(t.t, err)
	}

	readDebug := func(node, target string) (bool, error) {
		// 1. find the file
		pwd, err := os.Getwd()
		if err != nil {
			return false, err
		}

		pwd += "/.logs-tranche0/"
		entries, err := os.ReadDir(pwd)
		if err != nil {
			return false, err
		}

		var nodeEntry fs.DirEntry
		for _, entry := range entries {
			if strings.Contains(entry.Name(), node) {
				nodeEntry = entry
				break
			}
		}

		pwd += nodeEntry.Name()

		fileBytes, err := os.ReadFile(pwd)
		if err != nil {
			return false, err
		}

		fileString := string(fileBytes)
		if strings.Contains(fileString, target) {
			return true, nil
		}

		return false, fmt.Errorf("missed our chance")
	}

	// alice pays the first pay req - carol won't do the dance with bob
	go payFunc(net.Alice, payReqs[0])

	// wait until carol receives a commitsig
	var firstsig bool
	for i := 0; i < 20; i++ {
		firstsig, _ = readDebug("Carol", "Received CommitSig")
		if firstsig {
			break
		}
		time.Sleep(time.Millisecond * 100)
	}
	require.True(t.t, firstsig)

	// alice sends the next one
	time.Sleep(time.Second * 2)
	go payFunc(net.Alice, payReqs[1])

	// wait until bob starts the pending ticker
	var pendingtick bool
	for i := 0; i < 20; i++ {
		pendingtick, _ = readDebug("Bob", "PendingCommitTicker resumed")
		if pendingtick {
			break
		}
		time.Sleep(time.Millisecond * 100)
	}
	require.True(t.t, pendingtick)

	// alice sends the third one after 25 seconds
	time.Sleep(time.Second * 25)
	go payFunc(net.Alice, payReqs[2])

	// wait until the link goes down
	var dancefail bool
	for i := 0; i < 65; i++ {
		dancefail, _ = readDebug("Bob", "unable to complete dance")
		if dancefail {
			break
		}
		time.Sleep(time.Second)
	}
	require.True(t.t, dancefail)

	// Wait about 5 seconds for the mailbox fail to get locked on the alice
	// bob channel for the multihop payment from Alice.
	time.Sleep(time.Second * 5)

	// restart carol without the hodl.Revoke flag and get alice to attempt 2
	// more payments.
	carol.SetExtraArgs([]string{"--nolisten", "--minbackoff=1h"})
	if err := net.RestartNode(carol, nil); err != nil {
		t.Fatalf("Node restart failed: %v", err)
	}
	net.EnsureConnected(t.t, carol, net.Bob)

	// wait 2 seconds
	time.Sleep(time.Second * 2)

	go payFunc(net.Alice, payReqs[3])
	go payFunc(net.Alice, payReqs[4])

	var dupestone bool
	for i := 0; i < 40; i++ {
		dupestone, _ = readDebug("Bob", "duplicate keystone")
		if dupestone {
			break
		}
		time.Sleep(time.Millisecond * 100)
	}
	require.True(t.t, dupestone)

	// wait 30 seconds and try again
	time.Sleep(time.Second * 30)

	carol.SetExtraArgs([]string{"--nolisten", "--minbackoff=1h"})
	if err := net.RestartNode(carol, nil); err != nil {
		t.Fatalf("Node restart failed: %v", err)
	}
	net.EnsureConnected(t.t, carol, net.Bob)

	go payFunc(net.Alice, payReqs[5])
	go payFunc(net.Alice, payReqs[6])

	time.Sleep(time.Second * 10)
}
