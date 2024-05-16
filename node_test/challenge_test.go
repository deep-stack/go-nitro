package node_test

import (
	"context"
	"math/big"
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
	"github.com/statechannels/go-nitro/node/query"
	"github.com/statechannels/go-nitro/protocols"
	"github.com/statechannels/go-nitro/types"
)

const ChallengeDuration = 5

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
	balanceNodeA, _ := sim.BalanceAt(context.Background(), ethAccounts[0].From, latestBlock.Number())
	balanceNodeB, _ := sim.BalanceAt(context.Background(), ethAccounts[1].From, latestBlock.Number())
	t.Log("Balance of node A", balanceNodeA, "\nBalance of Node B", balanceNodeB)

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
	testhelpers.Assert(t, balanceA.Cmp(big.NewInt(ledgerChannelDeposit)) == 0, "BalanceA (%v) should be equal to ledgerChannelDeposit (%v)", balanceA, ledgerChannelDeposit)
	testhelpers.Assert(t, balanceB.Cmp(big.NewInt(ledgerChannelDeposit)) == 0, "BalanceB (%v) should be equal to ledgerChannelDeposit (%v)", balanceB, ledgerChannelDeposit)
}

func TestVirtualPaymentChannel(t *testing.T) {
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
	balanceNodeA, _ := sim.BalanceAt(context.Background(), ethAccounts[0].From, latestBlock.Number())
	balanceNodeB, _ := sim.BalanceAt(context.Background(), ethAccounts[1].From, latestBlock.Number())
	t.Log("Balance of node A", balanceNodeA, "\nBalance of Node B", balanceNodeB)

	// Create virtual channel
	virtualOutcome := initialPaymentOutcome(*nodeA.Address, *nodeB.Address, types.Address{})
	virtualResponse, _ := nodeA.CreatePaymentChannel([]common.Address{}, *nodeB.Address, ChallengeDuration, virtualOutcome)

	// Wait for objective to complete
	waitForObjectives(t, nodeA, nodeB, []node.Node{}, []protocols.ObjectiveId{virtualResponse.Id})
	checkPaymentChannel(t, virtualResponse.ChannelId, virtualOutcome, query.Open, nodeA, nodeB)

	// Make payment
	nodeA.Pay(virtualResponse.ChannelId, big.NewInt(int64(100)))

	// Wait for node B to recieve voucher
	nodeBVoucher := <- nodeB.ReceivedVouchers()
	t.Log("Voucher recieved", nodeBVoucher)

	targetFinalOutcome := finalPaymentOutcome(*nodeA.Address, *nodeB.Address, types.Address{}, 1, 100)
	checkPaymentChannel(t, virtualResponse.ChannelId, targetFinalOutcome, query.Open, nodeA, nodeB)

	signedLedgerState := getLatestSignedState(storeA, ledgerChannel)

	closeNode(t, &nodeB)

	// Node A calls challenge method
	challengerSig, _ := NitroAdjudicator.SignChallengeMessage(signedLedgerState.State(), ta.Alice.PrivateKey)
	challengeTx := protocols.NewChallengeTransaction(ledgerChannel, signedLedgerState, make([]state.SignedState, 0), challengerSig)
	err = testChainServiceA.SendTransaction(challengeTx)
	if err != nil {
		t.Error(err)
	}

	// Wait for challenge duration
	time.Sleep(time.Duration(ChallengeDuration) * time.Second)

	// Finalize Outcome
	sim.Commit()

	// Call Reclaim method
	signedUpdatedLedgerState := getLatestSignedState(storeA, ledgerChannel)
	signedStateHash, _ := signedUpdatedLedgerState.State().Hash()
	signedVirtualState := getVirtualSignedState(storeA, virtualResponse.ChannelId)
	virtualStateHash, _ := signedVirtualState.Hash()
	sourceOutcome := signedUpdatedLedgerState.State().Outcome
	sourceOb, _ := sourceOutcome.Encode()
	targetOutcome := signedVirtualState.Outcome
	targetOb, _ := targetOutcome.Encode()

	reclaimArgs := NitroAdjudicator.IMultiAssetHolderReclaimArgs{
		SourceChannelId: ledgerChannel,
		SourceStateHash: signedStateHash,
		SourceOutcomeBytes: sourceOb,
		SourceAssetIndex: common.Big0,
		IndexOfTargetInSource: common.Big2,
		TargetStateHash: virtualStateHash,
		TargetOutcomeBytes: targetOb,
		TargetAssetIndex: common.Big0,
	}

	reclaimTx := protocols.NewReclaimTransaction(ledgerChannel, reclaimArgs)
	err = testChainServiceA.SendTransaction(reclaimTx)
	if err != nil {
		t.Error(err)
	}

	reclaimedLedgerState := getLatestSignedState(storeA, ledgerChannel)

	// Node A calls transferAllAssets method
	transferTx := protocols.NewTransferAllTransaction(ledgerChannel, reclaimedLedgerState)
	err = testChainServiceA.SendTransaction(transferTx)
	if err != nil {
		t.Error(err)
	}

	latestBlock, _ = sim.BlockByNumber(context.Background(), nil)
	balanceANodeA, _ := sim.BalanceAt(context.Background(), ethAccounts[0].From, latestBlock.Number())
	balanceANodeB, _ := sim.BalanceAt(context.Background(), ethAccounts[1].From, latestBlock.Number())
	t.Log("Balance of node A", balanceANodeA, "\nBalance of Node B", balanceANodeB)

	// Check assets are liquidated
	latestBlock, _ = sim.BlockByNumber(context.Background(), nil)
	balanceA, _ := sim.BalanceAt(context.Background(), ta.Alice.Address(), latestBlock.Number())
	balanceB, _ := sim.BalanceAt(context.Background(), ta.Bob.Address(), latestBlock.Number())
	t.Log("Balance of A", balanceA, "\nBalance of B", balanceB)
	testhelpers.Assert(t, balanceA.Cmp(big.NewInt(ledgerChannelDeposit)) == 0, "BalanceA (%v) should be equal to ledgerChannelDeposit (%v)", balanceA, ledgerChannelDeposit)
	testhelpers.Assert(t, balanceB.Cmp(big.NewInt(ledgerChannelDeposit)) == 0, "BalanceB (%v) should be equal to ledgerChannelDeposit (%v)", balanceB, ledgerChannelDeposit)
}

func getLatestSignedState(store store.Store, id types.Destination) state.SignedState {
	consensusChannel, _ := store.GetConsensusChannelById(id)
	return consensusChannel.SupportedSignedState()
}

func getVirtualSignedState(store store.Store, id types.Destination) state.State {
	virtualChannel, _ := store.GetChannelById(id)
	virtualState, _ := virtualChannel.LatestSupportedState()
	return virtualState
}