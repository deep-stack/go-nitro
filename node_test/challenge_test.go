package node_test

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/statechannels/go-nitro/channel/state"
	ta "github.com/statechannels/go-nitro/internal/testactors"
	"github.com/statechannels/go-nitro/internal/testhelpers"
	"github.com/statechannels/go-nitro/node/engine/chainservice"
	NitroAdjudicator "github.com/statechannels/go-nitro/node/engine/chainservice/adjudicator"
	"github.com/statechannels/go-nitro/node/engine/messageservice"
	"github.com/statechannels/go-nitro/protocols"
	"github.com/statechannels/go-nitro/types"
)

const ChallengeDuration = 30

func TestChallenge(t *testing.T) {
	// Start the chain & deploy contract
	t.Log("Starting chain")
	sim, bindings, ethAccounts, err := chainservice.SetupSimulatedBackend(2)
	defer closeSimulatedChain(t, sim)
	if err != nil {
		t.Fatal(err)
	}

	// Create go-nitro nodes
	chainServiceA, _ := chainservice.NewSimulatedBackendChainService(sim, bindings, ethAccounts[0])
	chainServiceB, _ := chainservice.NewSimulatedBackendChainService(sim, bindings, ethAccounts[1])
	testChainServiceA, _ := chainservice.NewSimulatedBackendChainService(sim, bindings, ethAccounts[0])
	msgBroker := messageservice.NewBroker()
	dataFolder, cleanup := testhelpers.GenerateTempStoreFolder()
	defer cleanup()
	nodeA, _ := setupNode(ta.Alice.PrivateKey, chainServiceA, msgBroker, 0, dataFolder)
	defer closeNode(t, &nodeA)
	nodeB, _ := setupNode(ta.Bob.PrivateKey, chainServiceB, msgBroker, 0, dataFolder)

	// Create ledger channel
	ledgerChannel := openLedgerChannel(t, nodeA, nodeB, types.Address{}, 0)

	// Check balance of node
	latestBlock, _ := sim.BlockByNumber(context.Background(), nil)
	balanceNodeA, _ := sim.BalanceAt(context.Background(), ethAccounts[0].From, latestBlock.Number())
	balanceNodeB, _ := sim.BalanceAt(context.Background(), ethAccounts[1].From, latestBlock.Number())
	t.Log("Balance of node A", balanceNodeA, "\nBalance of Node B", balanceNodeB)

	// Close the node B
	closeNode(t, &nodeB)

	// Node A calls challenge method
	signedState := nodeA.GetSignedState(ledgerChannel)
	challengerSig, _ := NitroAdjudicator.SignChallengeMessage(signedState.State(), ta.Alice.PrivateKey)
	challengeTx := protocols.NewChallengeTransaction(ledgerChannel, signedState, make([]state.SignedState, 0), challengerSig)
	err = testChainServiceA.SendTransaction(challengeTx)
	if err != nil {
		t.Error(err)
	}

	// Wait for challenge duration
	time.Sleep(time.Duration(ChallengeDuration) * time.Second)

	// Finalize Outcome
	sim.Commit()

	// Node A calls transferAllAssets method
	transferTx := protocols.NewTransferAllTransaction(ledgerChannel, signedState)
	err = testChainServiceA.SendTransaction(transferTx)
	if err != nil {
		t.Error(err)
	}

	// Check assets are liquidated
	latestBlock, _ = sim.BlockByNumber(context.Background(), nil)
	balanceA, _ := sim.BalanceAt(context.Background(), ta.Alice.Address(), latestBlock.Number())
	balanceB, _ := sim.BalanceAt(context.Background(), ta.Bob.Address(), latestBlock.Number())
	t.Log("Balance of A", balanceA, "\nBalance of B", balanceB)
	testhelpers.Assert(t, balanceA.Cmp(big.NewInt(ledgerChannelDeposit)) == 0, "BalanceA (%v) should be equal to ledgerChannelDeposit (%v)", balanceA, ledgerChannelDeposit)
	testhelpers.Assert(t, balanceB.Cmp(big.NewInt(ledgerChannelDeposit)) == 0, "BalanceB (%v) should be equal to ledgerChannelDeposit (%v)", balanceB, ledgerChannelDeposit)
}