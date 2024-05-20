package node_test

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/statechannels/go-nitro/channel/state"
	"github.com/statechannels/go-nitro/cmd/utils"
	"github.com/statechannels/go-nitro/internal/chain"
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
	nodeBVoucher := <-nodeB.ReceivedVouchers()
	t.Log("Voucher recieved", nodeBVoucher)

	targetFinalOutcome := finalPaymentOutcome(*nodeA.Address, *nodeB.Address, types.Address{}, 1, 100)
	checkPaymentChannel(t, virtualResponse.ChannelId, targetFinalOutcome, query.Open, nodeA, nodeB)

	closeNode(t, &nodeB)

	// Call Reclaim method
	signedUpdatedLedgerState := getLatestSignedState(storeA, ledgerChannel)
	signedStateHash, _ := signedUpdatedLedgerState.State().Hash()
	virtualLatestState, _ := getVirtualSignedState(storeA, virtualResponse.ChannelId)
	virtualStateHash, _ := virtualLatestState.State().Hash()
	sourceOutcome := signedUpdatedLedgerState.State().Outcome
	sourceOb, _ := sourceOutcome.Encode()
	targetOutcome := virtualLatestState.State().Outcome
	targetOb, _ := targetOutcome.Encode()

	reclaimArgs := NitroAdjudicator.IMultiAssetHolderReclaimArgs{
		SourceChannelId:       ledgerChannel,
		SourceStateHash:       signedStateHash,
		SourceOutcomeBytes:    sourceOb,
		SourceAssetIndex:      common.Big0,
		IndexOfTargetInSource: common.Big2,
		TargetStateHash:       virtualStateHash,
		TargetOutcomeBytes:    targetOb,
		TargetAssetIndex:      common.Big0,
	}

	// Node A calls challenge method on virtual channel
	virtualChallengerSig, _ := NitroAdjudicator.SignChallengeMessage(virtualLatestState.State(), ta.Alice.PrivateKey)
	virtualChallengeTx := protocols.NewChallengeTransaction(virtualResponse.ChannelId, virtualLatestState, []state.SignedState{}, virtualChallengerSig)

	err = testChainServiceA.SendTransaction(virtualChallengeTx)
	if err != nil {
		t.Error(err)
	}

	// Wait for challenge duration
	time.Sleep(time.Duration(ChallengeDuration) * time.Second)

	// Finalize Outcome
	sim.Commit()

	// Node A calls challenge method
	challengerSig, _ := NitroAdjudicator.SignChallengeMessage(signedUpdatedLedgerState.State(), ta.Alice.PrivateKey)
	challengeTx := protocols.NewChallengeTransaction(ledgerChannel, signedUpdatedLedgerState, make([]state.SignedState, 0), challengerSig)
	err = testChainServiceA.SendTransaction(challengeTx)
	if err != nil {
		t.Error(err)
	}

	// Wait for challenge duration
	time.Sleep(time.Duration(ChallengeDuration) * time.Second)

	// Finalize Outcome
	sim.Commit()

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

func TestVirtualPaymentChannelUsingAnvil(t *testing.T) {
	// TODO: Remove after getting latest state for transferAllAssets transaction
	t.Skip()
	anvilCmd, _ := chain.StartAnvil()
	defer utils.StopCommands(anvilCmd)

	chainAuthToken := ""
	chainUrl := "ws://127.0.0.1:8545"
	aliceChainPk := "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
	bobChainPk := "59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"
	naAddress, vpaAddress, caAddress, _ := chain.DeployContracts(context.Background(), chainUrl, chainAuthToken, aliceChainPk)

	chainOptsAlice := chainservice.ChainOpts{
		ChainUrl:        chainUrl,
		ChainStartBlock: 0,
		ChainAuthToken:  chainAuthToken,
		ChainPk:         aliceChainPk,
		NaAddress:       naAddress,
		VpaAddress:      vpaAddress,
		CaAddress:       caAddress,
	}
	chainOptsBob := chainservice.ChainOpts{
		ChainUrl:        chainUrl,
		ChainStartBlock: 0,
		ChainAuthToken:  chainAuthToken,
		ChainPk:         bobChainPk,
		NaAddress:       naAddress,
		VpaAddress:      vpaAddress,
		CaAddress:       caAddress,
	}
	chainServiceA, _ := chainservice.NewEthChainService(chainOptsAlice)
	chainServiceB, _ := chainservice.NewEthChainService(chainOptsBob)
	testChainServiceA, _ := chainservice.NewEthChainService(chainOptsBob)

	// Create go-nitro nodes
	msgBroker := messageservice.NewBroker()
	dataFolder, cleanup := testhelpers.GenerateTempStoreFolder()
	defer cleanup()
	nodeA, storeA := setupNode(ta.Alice.PrivateKey, chainServiceA, msgBroker, 0, dataFolder)
	defer closeNode(t, &nodeA)
	nodeB, _ := setupNode(ta.Bob.PrivateKey, chainServiceB, msgBroker, 0, dataFolder)

	// Create ledger channel
	ledgerChannel := openLedgerChannel(t, nodeA, nodeB, types.Address{}, ChallengeDuration)

	// Create virtual channel
	virtualOutcome := initialPaymentOutcome(*nodeA.Address, *nodeB.Address, types.Address{})
	virtualResponse, _ := nodeA.CreatePaymentChannel([]common.Address{}, *nodeB.Address, ChallengeDuration, virtualOutcome)

	// Wait for objective to complete
	waitForObjectives(t, nodeA, nodeB, []node.Node{}, []protocols.ObjectiveId{virtualResponse.Id})
	// TODO: Debug checkPaymentChannel method
	// checkPaymentChannel(t, virtualResponse.ChannelId, virtualOutcome, query.Open, nodeA, nodeB)

	// _, oldSignedState  := getVirtualSignedState(storeA, virtualResponse.ChannelId)
	// oldLedgerState := getLatestSignedState(storeA, ledgerChannel)

	// Make payment
	nodeA.Pay(virtualResponse.ChannelId, big.NewInt(int64(100)))

	// Wait for node B to recieve voucher
	nodeBVoucher := <-nodeB.ReceivedVouchers()
	t.Log("Voucher recieved", nodeBVoucher)

	// TODO: Debug checkPaymentChannel method
	// targetFinalOutcome := finalPaymentOutcome(*nodeA.Address, *nodeB.Address, types.Address{}, 1, 100)
	// checkPaymentChannel(t, virtualResponse.ChannelId, targetFinalOutcome, query.Open, nodeA, nodeB)

	closeNode(t, &nodeB)

	// Call Reclaim method
	signedUpdatedLedgerState := getLatestSignedState(storeA, ledgerChannel)
	signedStateHash, _ := signedUpdatedLedgerState.State().Hash()
	virtualLatestState, _ := getVirtualSignedState(storeA, virtualResponse.ChannelId)
	virtualStateHash, _ := virtualLatestState.State().Hash()
	sourceOutcome := signedUpdatedLedgerState.State().Outcome
	sourceOb, _ := sourceOutcome.Encode()
	targetOutcome := virtualLatestState.State().Outcome
	targetOb, _ := targetOutcome.Encode()

	reclaimArgs := NitroAdjudicator.IMultiAssetHolderReclaimArgs{
		SourceChannelId:       ledgerChannel,
		SourceStateHash:       signedStateHash,
		SourceOutcomeBytes:    sourceOb,
		SourceAssetIndex:      common.Big0,
		IndexOfTargetInSource: common.Big2,
		TargetStateHash:       virtualStateHash,
		TargetOutcomeBytes:    targetOb,
		TargetAssetIndex:      common.Big0,
	}

	// Node A calls challenge method on virtual channel
	virtualChallengerSig, _ := NitroAdjudicator.SignChallengeMessage(virtualLatestState.State(), ta.Alice.PrivateKey)
	virtualChallengeTx := protocols.NewChallengeTransaction(virtualResponse.ChannelId, virtualLatestState, []state.SignedState{}, virtualChallengerSig)
	err := testChainServiceA.SendTransaction(virtualChallengeTx)
	if err != nil {
		t.Error(err)
	}

	// Wait for challenge duration
	time.Sleep(time.Duration(ChallengeDuration) * time.Second)

	// Node A calls challenge method on ledger channel
	challengerSig, _ := NitroAdjudicator.SignChallengeMessage(signedUpdatedLedgerState.State(), ta.Alice.PrivateKey)
	challengeTx := protocols.NewChallengeTransaction(ledgerChannel, signedUpdatedLedgerState, make([]state.SignedState, 0), challengerSig)
	err = testChainServiceA.SendTransaction(challengeTx)
	if err != nil {
		t.Error(err)
	}

	// Wait for challenge duration
	time.Sleep(time.Duration(ChallengeDuration) * time.Second)

	reclaimTx := protocols.NewReclaimTransaction(ledgerChannel, reclaimArgs)
	err = testChainServiceA.SendTransaction(reclaimTx)
	if err != nil {
		t.Error(err)
	}

	// TODO: Use correct state for transferAllAssets transaction
	// Node A calls transferAllAssets method
	transferTx := protocols.NewTransferAllTransaction(ledgerChannel, signedUpdatedLedgerState)
	err = testChainServiceA.SendTransaction(transferTx)
	if err != nil {
		t.Error(err)
	}
}

func getLatestSignedState(store store.Store, id types.Destination) state.SignedState {
	consensusChannel, _ := store.GetConsensusChannelById(id)
	return consensusChannel.SupportedSignedState()
}

func getVirtualSignedState(store store.Store, id types.Destination) (state.SignedState, state.SignedState) {
	virtualChannel, _ := store.GetChannelById(id)
	virtualSignedState, _ := virtualChannel.LatestSignedState()
	virtualPostfundState := virtualChannel.SignedPostFundState()
	return virtualSignedState, virtualPostfundState
}
