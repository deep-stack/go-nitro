package node_test

import (
	"context"
	"fmt"
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/statechannels/go-nitro/channel/state"
	ta "github.com/statechannels/go-nitro/internal/testactors"
	"github.com/statechannels/go-nitro/internal/testhelpers"
	"github.com/statechannels/go-nitro/node"
	"github.com/statechannels/go-nitro/node/engine/chainservice"
	NitroAdjudicator "github.com/statechannels/go-nitro/node/engine/chainservice/adjudicator"
	"github.com/statechannels/go-nitro/node/engine/messageservice"
	"github.com/statechannels/go-nitro/node/engine/store"
	"github.com/statechannels/go-nitro/protocols"
	"github.com/statechannels/go-nitro/types"
)

const ChallengeDuration = 60

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
	nodeA, storeA := setupNode(ta.Alice.PrivateKey, chainServiceA, msgBroker, 0, dataFolder)
	defer closeNode(t, &nodeA)
	nodeB, _ := setupNode(ta.Bob.PrivateKey, chainServiceB, msgBroker, 0, dataFolder)

	// Create ledger channel
	ledgerChannel := openLedgerChannel(t, nodeA, nodeB, types.Address{}, ChallengeDuration)

	// Check balance of node
	latestBlock, _ := sim.BlockByNumber(context.Background(), nil)
	balanceNodeA, _ := sim.BalanceAt(context.Background(), ta.Alice.Address(), latestBlock.Number())
	balanceNodeB, _ := sim.BalanceAt(context.Background(), ta.Bob.Address(), latestBlock.Number())
	t.Log("Balance of node A", balanceNodeA, "\nBalance of Node B", balanceNodeB)
	testhelpers.Assert(t, balanceNodeA.Int64() == 0, "BalanceA should be zero")
	testhelpers.Assert(t, balanceNodeB.Int64() == 0, "BalanceB should be zero")

	// Close the node B
	closeNode(t, &nodeB)

	// Node A calls challenge method
	signedState := getLatestSignedState(storeA, ledgerChannel)
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
	// TODO: Update off chain states

	// Check assets are liquidated
	latestBlock, _ = sim.BlockByNumber(context.Background(), nil)
	balanceA, _ := sim.BalanceAt(context.Background(), ta.Alice.Address(), latestBlock.Number())
	balanceB, _ := sim.BalanceAt(context.Background(), ta.Bob.Address(), latestBlock.Number())
	t.Log("Balance of A", balanceA, "\nBalance of B", balanceB)
	// Assert balance equals ledger channel deposit since no payment has been made
	testhelpers.Assert(t, balanceA.Cmp(big.NewInt(ledgerChannelDeposit)) == 0, "BalanceA (%v) should be equal to ledgerChannelDeposit (%v)", balanceA, ledgerChannelDeposit)
	testhelpers.Assert(t, balanceB.Cmp(big.NewInt(ledgerChannelDeposit)) == 0, "BalanceB (%v) should be equal to ledgerChannelDeposit (%v)", balanceB, ledgerChannelDeposit)
}

func TestCheckpoint(t *testing.T) {
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
	msgBroker := messageservice.NewBroker()
	dataFolder, cleanup := testhelpers.GenerateTempStoreFolder()
	defer cleanup()
	nodeA, storeA := setupNode(ta.Alice.PrivateKey, chainServiceA, msgBroker, 0, dataFolder)
	defer closeNode(t, &nodeA)
	nodeB, storeB := setupNode(ta.Bob.PrivateKey, chainServiceB, msgBroker, 0, dataFolder)

	// Seperate chain service to listen for events
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	eventChan := make(chan chainservice.Event)
	testChainServiceA, _ := chainservice.NewSimulatedBackendChainService(sim, bindings, ethAccounts[0])
	out := testChainServiceA.EventFeed()
	eventTypes := map[reflect.Type]struct{}{
		reflect.TypeOf(chainservice.ChallengeRegisteredEvent{}): {},
		reflect.TypeOf(chainservice.ChallengeClearedEvent{}):    {},
	}
	go eventListener(ctx, out, eventChan, eventTypes)

	// Create ledger channel and check balance of node
	ledgerChannel := openLedgerChannel(t, nodeA, nodeB, types.Address{}, ChallengeDuration)
	latestBlock, _ := sim.BlockByNumber(context.Background(), nil)
	balanceNodeA, _ := sim.BalanceAt(context.Background(), ethAccounts[0].From, latestBlock.Number())
	balanceNodeB, _ := sim.BalanceAt(context.Background(), ethAccounts[1].From, latestBlock.Number())
	t.Log("Balance of node A", balanceNodeA, "\nBalance of Node B", balanceNodeB)

	// Store current state
	oldState := getLatestSignedState(storeA, ledgerChannel)

	// Conduct virtual fund and virtual defund
	virtualOutcome := initialPaymentOutcome(*nodeA.Address, *nodeB.Address, common.BigToAddress(common.Big0))
	response, err := nodeA.CreatePaymentChannel([]common.Address{}, *nodeB.Address, ChallengeDuration, virtualOutcome)
	if err != nil {
		t.Error(err)
	}
	waitForObjectives(t, nodeA, nodeB, []node.Node{}, []protocols.ObjectiveId{response.Id})
	virtualDefundResponse, err := nodeA.ClosePaymentChannel(response.ChannelId)
	if err != nil {
		t.Error(err)
	}
	waitForObjectives(t, nodeA, nodeB, []node.Node{}, []protocols.ObjectiveId{virtualDefundResponse})

	// Store current state
	newState := getLatestSignedState(storeB, ledgerChannel)

	// Node A call challenge method using old state
	challengerSig, _ := NitroAdjudicator.SignChallengeMessage(oldState.State(), ta.Alice.PrivateKey)
	challengeTx := protocols.NewChallengeTransaction(ledgerChannel, oldState, make([]state.SignedState, 0), challengerSig)
	err = chainServiceA.SendTransaction(challengeTx)
	if err != nil {
		t.Error(err)
	}

	// Listen for challenge registered event
	challengeEvent := <-eventChan
	t.Log("Challenge registed event received", challengeEvent)

	// Node B calls checkpoint method using new state
	checkpointTx := protocols.NewCheckpointTransaction(ledgerChannel, newState, make([]state.SignedState, 0))
	err = chainServiceB.SendTransaction(checkpointTx)
	if err != nil {
		t.Error(err)
	}

	// Listen for challenge cleared event
	challengeEvent = <-eventChan
	t.Log("Challenge cleared event received", challengeEvent)
}

func eventListener(ctx context.Context, eventChannel <-chan chainservice.Event, challengeEventChan chan chainservice.Event, eventTypes map[reflect.Type]struct{}) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-eventChannel:
			if _, ok := eventTypes[reflect.TypeOf(event)]; ok {
				fmt.Println("Processing events")
				challengeEventChan <- event
			} else {
				fmt.Println("Ignoring other events")
			}
		}
	}
}

func getLatestSignedState(store store.Store, id types.Destination) state.SignedState {
	consensusChannel, _ := store.GetConsensusChannelById(id)
	return consensusChannel.SupportedSignedState()
}
