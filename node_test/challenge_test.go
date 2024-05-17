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
	const ChallengeDuration = 5

	sim, ethAccounts, nodeA, nodeB, storeA, _, chainServiceA, _, _, cleanup := setupTestNodes(t)
	defer closeSimulatedChain(t, sim)
	defer closeNode(t, &nodeA)
	defer cleanup()

	// Create ledger channel
	ledgerChannel := openLedgerChannel(t, nodeA, nodeB, types.Address{}, ChallengeDuration)
	getBalance(t, sim, ethAccounts[0].From)
	getBalance(t, sim, ethAccounts[1].From)

	// Close the node B
	closeNode(t, &nodeB)

	// Node A calls challenge method
	signedState := getLatestSignedState(storeA, ledgerChannel)
	sendChallengeTransaction(t, signedState, ta.Alice.PrivateKey, ledgerChannel, chainServiceA)

	// Wait for challenge duration
	time.Sleep(time.Duration(ChallengeDuration) * time.Second)

	// Finalize Outcome
	sim.Commit()

	// Node A calls transferAllAssets method
	sendTransferTransaction(t, signedState, ledgerChannel, chainServiceA)

	// TODO: Update off chain states

	// Check assets are liquidated
	balanceA := getBalance(t, sim, ta.Alice.Address())
	balanceB := getBalance(t, sim, ta.Bob.Address())
	testhelpers.Assert(t, balanceA.Cmp(big.NewInt(ledgerChannelDeposit)) == 0, "BalanceA (%v) should be equal to ledgerChannelDeposit (%v)", balanceA, ledgerChannelDeposit)
	testhelpers.Assert(t, balanceB.Cmp(big.NewInt(ledgerChannelDeposit)) == 0, "BalanceB (%v) should be equal to ledgerChannelDeposit (%v)", balanceB, ledgerChannelDeposit)
}

func TestCheckpoint(t *testing.T) {
	const ChallengeDuration = 31

	sim, ethAccounts, nodeA, nodeB, storeA, storeB, chainServiceA, chainServiceB, testChainServiceA, cleanup := setupTestNodes(t)
	defer closeSimulatedChain(t, sim)
	defer closeNode(t, &nodeA)
	defer closeNode(t, &nodeB)
	defer cleanup()
	defer testChainServiceA.Close()

	// Create ledger channel and check balance of node
	ledgerChannel := openLedgerChannel(t, nodeA, nodeB, types.Address{}, ChallengeDuration)
	getBalance(t, sim, ethAccounts[0].From)
	getBalance(t, sim, ethAccounts[1].From)

	// Store current state
	oldState := getLatestSignedState(storeA, ledgerChannel)

	// Create virtual channel, make payment and close virtual channel
	makePayment(t, nodeA, nodeB, ChallengeDuration)

	// Store current state
	newState := getLatestSignedState(storeB, ledgerChannel)

	// Node A calls challenge method using old state
	sendChallengeTransaction(t, oldState, ta.Alice.PrivateKey, ledgerChannel, chainServiceA)

	// Listen for challenge registered event
	event := eventListener(t, testChainServiceA.EventFeed(), chainservice.ChallengeRegisteredEvent{})
	t.Log("Challenge registed event received", event)

	// Node B calls checkpoint method using new state
	checkpointTx := protocols.NewCheckpointTransaction(ledgerChannel, newState, make([]state.SignedState, 0))
	err := chainServiceB.SendTransaction(checkpointTx)
	if err != nil {
		t.Error(err)
	}

	// Listen for challenge cleared event
	event = eventListener(t, testChainServiceA.EventFeed(), chainservice.ChallengeClearedEvent{})
	t.Log("Challenge cleared event received", event)
	_, ok := event.(chainservice.ChallengeClearedEvent)
	testhelpers.Assert(t, ok, "Expect challenge cleared event")
}

func TestCounterChallenge(t *testing.T) {
	const ChallengeDuration = 31

	sim, ethAccounts, nodeA, nodeB, storeA, storeB, chainServiceA, chainServiceB, testChainServiceA, cleanup := setupTestNodes(t)
	defer closeSimulatedChain(t, sim)
	defer closeNode(t, &nodeA)
	defer closeNode(t, &nodeB)
	defer cleanup()
	defer testChainServiceA.Close()

	// Create ledger channel and check balance of node
	ledgerChannel := openLedgerChannel(t, nodeA, nodeB, types.Address{}, ChallengeDuration)
	getBalance(t, sim, ethAccounts[0].From)
	getBalance(t, sim, ethAccounts[1].From)

	// Store current state
	oldState := getLatestSignedState(storeA, ledgerChannel)

	// Create virtual channel, make payment and close virtual channel
	makePayment(t, nodeA, nodeB, ChallengeDuration)

	// Store current state
	newState := getLatestSignedState(storeB, ledgerChannel)

	// Node A calls challenge method using old state
	sendChallengeTransaction(t, oldState, ta.Alice.PrivateKey, ledgerChannel, chainServiceA)

	// Listen for challenge registered event
	event := eventListener(t, testChainServiceA.EventFeed(), chainservice.ChallengeRegisteredEvent{})
	t.Log("Challenge registed event received", event)
	_, ok := event.(chainservice.ChallengeRegisteredEvent)
	testhelpers.Assert(t, ok, "Expect challenge registered event")

	// Node B calls challenge method using new state
	sendChallengeTransaction(t, newState, ta.Bob.PrivateKey, ledgerChannel, chainServiceB)

	// Listen for challenge register event
	event = eventListener(t, testChainServiceA.EventFeed(), chainservice.ChallengeRegisteredEvent{})
	t.Log("Challenge registed event received", event)
	_, ok = event.(chainservice.ChallengeRegisteredEvent)
	testhelpers.Assert(t, ok, "Expect challenge registered event")

	// Wait for challenge duration
	time.Sleep(time.Duration(ChallengeDuration) * time.Second)

	// Finalize Outcome
	sim.Commit()

	// Node B calls transferAllAssets method using new state
	sendTransferTransaction(t, newState, ledgerChannel, chainServiceB)

	// Check assets are liquidated
	balanceA := getBalance(t, sim, ta.Alice.Address())
	balanceB := getBalance(t, sim, ta.Bob.Address())
	testhelpers.Assert(t, balanceA.Cmp(big.NewInt(ledgerChannelDeposit-virtualChannelDeposit)) == 0, "BalanceA (%v) should be equal to (%v)", balanceA, ledgerChannelDeposit-virtualChannelDeposit)
	testhelpers.Assert(t, balanceB.Cmp(big.NewInt(ledgerChannelDeposit+virtualChannelDeposit)) == 0, "BalanceB (%v) should be equal to (%v)", balanceB, ledgerChannelDeposit+virtualChannelDeposit)
}

func makePayment(t *testing.T, nodeA node.Node, nodeB node.Node, challengeDuration uint32) {
	virtualOutcome := initialPaymentOutcome(*nodeA.Address, *nodeB.Address, common.BigToAddress(common.Big0))
	response, err := nodeA.CreatePaymentChannel([]common.Address{}, *nodeB.Address, challengeDuration, virtualOutcome)
	if err != nil {
		t.Error(err)
	}
	waitForObjectives(t, nodeA, nodeB, []node.Node{}, []protocols.ObjectiveId{response.Id})
	nodeA.Pay(response.ChannelId, big.NewInt(virtualChannelDeposit))
	nodeBVoucher := <-nodeB.ReceivedVouchers()
	t.Logf("Voucher recieved %+v", nodeBVoucher)
	virtualDefundResponse, err := nodeA.ClosePaymentChannel(response.ChannelId)
	if err != nil {
		t.Error(err)
	}
	waitForObjectives(t, nodeA, nodeB, []node.Node{}, []protocols.ObjectiveId{virtualDefundResponse})
}

func getBalance(t *testing.T, sim chainservice.SimulatedChain, address common.Address) *big.Int {
	ctx := context.Background()
	latestBlock, _ := sim.BlockByNumber(ctx, nil)
	balance, err := sim.BalanceAt(ctx, address, latestBlock.Number())
	if err != nil {
		t.Error(err)
	}
	t.Logf("Balance of %s is %s\n", address, balance.String())
	return balance
}

func sendChallengeTransaction(t *testing.T, signedState state.SignedState, privateKey []byte, ledgerChannel types.Destination, chainService chainservice.ChainService) {
	challengerSig, _ := NitroAdjudicator.SignChallengeMessage(signedState.State(), privateKey)
	challengeTx := protocols.NewChallengeTransaction(ledgerChannel, signedState, make([]state.SignedState, 0), challengerSig)
	err := chainService.SendTransaction(challengeTx)
	if err != nil {
		t.Error(err)
	}
}

func sendTransferTransaction(t *testing.T, signedState state.SignedState, ledgerChannel types.Destination, chainService chainservice.ChainService) {
	transferTx := protocols.NewTransferAllTransaction(ledgerChannel, signedState)
	err := chainService.SendTransaction(transferTx)
	if err != nil {
		t.Error(err)
	}
}

func setupTestNodes(t *testing.T) (
	chainservice.SimulatedChain, []*bind.TransactOpts, node.Node, node.Node, store.Store, store.Store, chainservice.ChainService, chainservice.ChainService, chainservice.ChainService, func(),
) {
	// Start the chain & deploy contract
	t.Log("Starting chain")
	sim, bindings, ethAccounts, err := chainservice.SetupSimulatedBackend(2)
	if err != nil {
		t.Fatal(err)
	}

	// Create go-nitro nodes
	chainServiceA, _ := chainservice.NewSimulatedBackendChainService(sim, bindings, ethAccounts[0])
	chainServiceB, _ := chainservice.NewSimulatedBackendChainService(sim, bindings, ethAccounts[1])
	testChainServiceA, _ := chainservice.NewSimulatedBackendChainService(sim, bindings, ethAccounts[0])
	msgBroker := messageservice.NewBroker()
	dataFolder, cleanup := testhelpers.GenerateTempStoreFolder()
	nodeA, storeA := setupNode(ta.Alice.PrivateKey, chainServiceA, msgBroker, 0, dataFolder)
	nodeB, storeB := setupNode(ta.Bob.PrivateKey, chainServiceB, msgBroker, 0, dataFolder)

	return sim, ethAccounts, nodeA, nodeB, storeA, storeB, chainServiceA, chainServiceB, testChainServiceA, cleanup
}

func eventListener(t *testing.T, eventChannel <-chan chainservice.Event, eventType chainservice.Event) chainservice.Event {
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
