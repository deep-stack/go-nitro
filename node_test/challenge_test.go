package node_test

import (
	"context"
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
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

func TestChallenge(t *testing.T) {
	const challengeDuration = 5

	// Start the chain & deploy contract
	t.Log("Starting chain")
	sim, bindings, ethAccounts, err := chainservice.SetupSimulatedBackend(2)
	defer closeSimulatedChain(t, sim)
	if err != nil {
		t.Fatal(err)
	}

	// Create go-nitro nodes
	msgBroker := messageservice.NewBroker()
	dataFolder, cleanup := testhelpers.GenerateTempStoreFolder()
	defer cleanup()
	nodeA, storeA, _ := setupTestNode(sim, bindings, ethAccounts[0], ta.Alice.PrivateKey, msgBroker, dataFolder)
	nodeB, _, _ := setupTestNode(sim, bindings, ethAccounts[1], ta.Bob.PrivateKey, msgBroker, dataFolder)
	defer closeNode(t, &nodeA)

	testChainServiceA, _ := chainservice.NewSimulatedBackendChainService(sim, bindings, ethAccounts[0])
	defer testChainServiceA.Close()

	// Create ledger channel
	ledgerChannel := openLedgerChannel(t, nodeA, nodeB, types.Address{}, challengeDuration)

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
	time.Sleep(time.Duration(challengeDuration) * time.Second)

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
	const challengeDuration = 31

	// Start the chain & deploy contract
	t.Log("Starting chain")
	sim, bindings, ethAccounts, err := chainservice.SetupSimulatedBackend(2)
	defer closeSimulatedChain(t, sim)
	if err != nil {
		t.Fatal(err)
	}

	// Create go-nitro nodes
	msgBroker := messageservice.NewBroker()
	dataFolder, cleanup := testhelpers.GenerateTempStoreFolder()
	defer cleanup()
	nodeA, storeA, chainServiceA := setupTestNode(sim, bindings, ethAccounts[0], ta.Alice.PrivateKey, msgBroker, dataFolder)
	nodeB, storeB, chainServiceB := setupTestNode(sim, bindings, ethAccounts[1], ta.Bob.PrivateKey, msgBroker, dataFolder)
	defer closeNode(t, &nodeA)
	defer closeNode(t, &nodeB)

	// Seperate chain service to listen for events
	testChainServiceA, _ := chainservice.NewSimulatedBackendChainService(sim, bindings, ethAccounts[0])
	defer testChainServiceA.Close()

	// Create ledger channel and check balance of node
	ledgerChannel := openLedgerChannel(t, nodeA, nodeB, types.Address{}, challengeDuration)

	// Store current state
	oldState := getLatestSignedState(storeA, ledgerChannel)

	// Conduct virtual fund and virtual defund
	virtualOutcome := initialPaymentOutcome(*nodeA.Address, *nodeB.Address, common.BigToAddress(common.Big0))
	response, err := nodeA.CreatePaymentChannel([]common.Address{}, *nodeB.Address, challengeDuration, virtualOutcome)
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
	event := waitForEvent(t, testChainServiceA.EventFeed(), chainservice.ChallengeRegisteredEvent{})
	t.Log("Challenge registed event received", event)

	// Node B calls checkpoint method using new state
	checkpointTx := protocols.NewCheckpointTransaction(ledgerChannel, newState, make([]state.SignedState, 0))
	err = chainServiceB.SendTransaction(checkpointTx)
	if err != nil {
		t.Error(err)
	}

	// // Listen for challenge cleared event
	event = waitForEvent(t, testChainServiceA.EventFeed(), chainservice.ChallengeClearedEvent{})
	t.Log("Challenge cleared event received", event)
	challengeClearedEvent, ok := event.(chainservice.ChallengeClearedEvent)
	testhelpers.Assert(t, ok, "Expect challenge cleared event")
	testhelpers.Assert(t, challengeClearedEvent.ChannelID() == ledgerChannel, "Channel ID mismatch")

	// Node A attempts to liquidate the asset after the challenge duration, but the attempt fails because the outcome has not been finalized
	time.Sleep(time.Duration(challengeDuration) * time.Second)
	sim.Commit()
	transferTx := protocols.NewTransferAllTransaction(ledgerChannel, oldState)
	err = chainServiceA.SendTransaction(transferTx)
	testhelpers.Assert(t, err.Error() == "execution reverted: Channel not finalized.", "Expects execution reverted error")
}

func setupTestNode(sim chainservice.SimulatedChain, bindings chainservice.Bindings, ethAccount *bind.TransactOpts, privateKey []byte, msgBroker messageservice.Broker, dataFolder string) (node.Node, store.Store, chainservice.ChainService) {
	chainService, _ := chainservice.NewSimulatedBackendChainService(sim, bindings, ethAccount)
	node, store := setupNode(privateKey, chainService, msgBroker, 0, dataFolder)
	return node, store, chainService
}

func waitForEvent(t *testing.T, eventChannel <-chan chainservice.Event, eventType chainservice.Event) chainservice.Event {
	for event := range eventChannel {
		if reflect.TypeOf(event) == reflect.TypeOf(eventType) {
			return event
		} else {
			t.Log("Ignoring other events")
		}
	}
	return nil
}

func getLatestSignedState(store store.Store, id types.Destination) state.SignedState {
	consensusChannel, _ := store.GetConsensusChannelById(id)
	return consensusChannel.SupportedSignedState()
}
