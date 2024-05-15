package node_test // import "github.com/statechannels/go-nitro/node_test"

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

const challengeDuration uint32 = 30

func TestChallenge(t *testing.T) {
	t.Log("Test challenge protocol")

	// Start the chain & deploy contract
	sim, bindings, ethAccounts, err := chainservice.SetupSimulatedBackend(2)
	defer closeSimulatedChain(t, sim)
	if err != nil {
		t.Fatal(err)
	}

	// Create go-nitro nodes
	chainServiceA, _ := chainservice.NewSimulatedBackendChainService(sim, bindings, ethAccounts[0])
	chainServiceB, _ := chainservice.NewSimulatedBackendChainService(sim, bindings, ethAccounts[1])

	chainServiceStandalone, _ := chainservice.NewSimulatedBackendChainService(sim, bindings, ethAccounts[0])

	msgBroker := messageservice.NewBroker()

	dataFolder, cleanup := testhelpers.GenerateTempStoreFolder()
	defer cleanup()

	nodeA, _ := setupNode(ta.Alice.PrivateKey, chainServiceA, msgBroker, 0, dataFolder)
	defer closeNode(t, &nodeA)

	nodeB, _ := setupNode(ta.Bob.PrivateKey, chainServiceB, msgBroker, 0, dataFolder)

	// Create ledger channel
	ledgerChannel := openLedgerChannel(t, nodeA, nodeB, types.Address{}, 0)

	// Check balance of node
	balanceNodeA, _ := sim.BalanceAt(context.Background(), ethAccounts[0].From, big.NewInt(6))
	balanceNodeB, _ := sim.BalanceAt(context.Background(), ethAccounts[1].From, big.NewInt(6))
	t.Log("Balance of node A", balanceNodeA, "\nBalance of Node B", balanceNodeB)

	// Close the node B
	closeNode(t, &nodeB)

	// Node A call challenge method
	signedState := nodeA.GetSignedState(ledgerChannel)
	challengerSig, _ := NitroAdjudicator.SignChallengeMessage(signedState.State(), ta.Alice.PrivateKey)
	challengeTx := protocols.NewChallengeTransaction(ledgerChannel, signedState, make([]state.SignedState, 0), challengerSig)
	err = chainServiceStandalone.SendTransaction(challengeTx)
	if err != nil {
		t.Error(err)
	}

	// wait for challenge duration
	time.Sleep(time.Duration(challengeDuration) * time.Second)

	// Finalize Outcome
	sim.Commit()

	// Node A call transfer method
	transferTx := protocols.NewTransferTransaction(ledgerChannel, signedState)
	err = chainServiceStandalone.SendTransaction(transferTx)
	if err != nil {
		t.Error(err)
	}

	// Check assets are liquidated
	balanceA, _ := sim.BalanceAt(context.Background(), ta.Alice.Address(), big.NewInt(12))
	balanceB, _ := sim.BalanceAt(context.Background(), ta.Bob.Address(), big.NewInt(12))
	t.Log("Balance of A", balanceA, "\nBalance of B", balanceB)
}
