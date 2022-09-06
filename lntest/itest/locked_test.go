package itest

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lntest"
	"github.com/stretchr/testify/require"
)

func testFL(net *lntest.NetworkHarness, t *harnessTest) {
	ctxb := context.Background()

	chanAmt := btcutil.Amount(1_000_000)

	p := lntest.OpenChannelParams{
		Amt: chanAmt,
	}

	c, err := net.OpenChannel(net.Alice, net.Bob, p)
	require.NoError(t.t, err)
	_ = c

	// disconnect and connect in a goroutine continuously
	go func() {
		after := time.After(time.Second * 10)
		for {
			select {
			case <-after:
				return
			default:
			}
			net.DisconnectNodes(net.Alice, net.Bob)
			time.Sleep(time.Millisecond * 50)
			net.EnsureConnected(t.t, net.Alice, net.Bob)
			time.Sleep(time.Millisecond * 50)
		}
	}()

	// mine block x3 and reconnect during
	mineBlocksSlow(t, net, 3, 0)

	time.Sleep(time.Second * 10)

	net.EnsureConnected(t.t, net.Alice, net.Bob)

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
	_ = readDebug

	// var success bool
	// for i := 0; i < 10; i++ {
	// 	// success, _ = readDebug("Alice", "peer quit signal")
	// 	// success, _ = readDebug("Alice", "unable to read message")
	// 	// if success {
	// 	// break
	// 	// }
	// 	success, _ = readDebug("Bob", "peer quit signal")
	// 	// success, _ = readDebug("Bob", "unable to read message")
	// 	if success {
	// 		break
	// 	}
	// 	time.Sleep(time.Millisecond * 250)
	// }
	// if success {
	// 	t.t.Fatalf("fail to let flakehunter know")
	// }

	time.Sleep(time.Second * 20)

	req := &lnrpc.ChannelGraphRequest{
		IncludeUnannounced: true,
	}

	// check that both nodes have both edges
	aGraph, err := net.Alice.DescribeGraph(ctxb, req)
	require.NoError(t.t, err)

	require.Equal(t.t, len(aGraph.Edges), 1)
	edge := aGraph.Edges[0]
	require.NotNil(t.t, edge.Node1Policy)
	require.NotNil(t.t, edge.Node2Policy)

	bGraph, err := net.Bob.DescribeGraph(ctxb, req)
	require.NoError(t.t, err)
	edge = bGraph.Edges[0]
	require.NotNil(t.t, edge.Node1Policy)
	require.NotNil(t.t, edge.Node2Policy)
}
