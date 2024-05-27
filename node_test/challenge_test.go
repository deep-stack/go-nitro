package node_test

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/statechannels/go-nitro/channel/state"
	"github.com/statechannels/go-nitro/channel/state/outcome"
	"github.com/statechannels/go-nitro/internal/testactors"
	ta "github.com/statechannels/go-nitro/internal/testactors"
	"github.com/statechannels/go-nitro/internal/testhelpers"
	"github.com/statechannels/go-nitro/node"
	"github.com/statechannels/go-nitro/node/engine/chainservice"
	NitroAdjudicator "github.com/statechannels/go-nitro/node/engine/chainservice/adjudicator"
	"github.com/statechannels/go-nitro/node/engine/store"
	"github.com/statechannels/go-nitro/node/query"
	"github.com/statechannels/go-nitro/protocols"
	"github.com/statechannels/go-nitro/types"
)

type VoucherAmountSignature struct {
	Amount    *big.Int
	Signature NitroAdjudicator.INitroTypesSignature
}

func TestChallenge(t *testing.T) {
	tc := TestCase{
		Description:       "Challenge test",
		Chain:             AnvilChain,
		MessageService:    TestMessageService,
		MessageDelay:      0,
		LogName:           "Challenge_test",
		ChallengeDuration: 5,
		Participants: []TestParticipant{
			{StoreType: MemStore, Actor: testactors.Alice},
			{StoreType: MemStore, Actor: testactors.Bob},
		},
	}

	dataFolder, cleanup := testhelpers.GenerateTempStoreFolder()
	defer cleanup()

	infra := setupSharedInfra(tc)
	defer infra.Close(t)

	// Create go-nitro nodes
	nodeA, _, _, storeA, chainServiceA := setupIntegrationNode(tc, tc.Participants[0], infra, []string{}, dataFolder)
	defer nodeA.Close()

	nodeB, _, _, _, _ := setupIntegrationNode(tc, tc.Participants[1], infra, []string{}, dataFolder)

	// Separate chain service to listen for events
	testChainServiceA := setupChainService(tc, tc.Participants[0], infra)
	defer testChainServiceA.Close()

	// Create ledger channel
	ledgerChannel := openLedgerChannel(t, nodeA, nodeB, types.Address{}, uint32(tc.ChallengeDuration))

	// Check balance of node
	balanceNodeA, _ := infra.anvilChain.GetAccountBalance(tc.Participants[0].Address())
	balanceNodeB, _ := infra.anvilChain.GetAccountBalance(tc.Participants[1].Address())
	t.Log("Balance of Alice", balanceNodeA, "\nBalance of Bob", balanceNodeB)
	testhelpers.Assert(t, balanceNodeA.Int64() == 0, "Balance of Alice should be zero")
	testhelpers.Assert(t, balanceNodeB.Int64() == 0, "Balance of Bob should be zero")

	// Close the Bob's node
	closeNode(t, &nodeB)

	// Alice calls challenge method
	signedState := getLatestSignedState(storeA, ledgerChannel)
	sendChallengeTransaction(t, signedState, tc.Participants[0].PrivateKey, ledgerChannel, testChainServiceA)

	// Listen for challenge registered event
	event := waitForEvent(t, testChainServiceA.EventFeed(), chainservice.ChallengeRegisteredEvent{})
	challengeRegisteredEvent, ok := event.(chainservice.ChallengeRegisteredEvent)
	testhelpers.Assert(t, ok, "Expected challenge registered event")

	time.Sleep(time.Duration(tc.ChallengeDuration) * time.Second)
	latestBlock, _ := infra.anvilChain.GetLatestBlock()
	testhelpers.Assert(t, challengeRegisteredEvent.FinalizesAt.Uint64() <= latestBlock.Header().Time, "Expected channel to be finalized")

	// Alice calls transferAllAssets method
	transferTx := protocols.NewTransferAllTransaction(ledgerChannel, signedState)
	err := chainServiceA.SendTransaction(transferTx)
	if err != nil {
		t.Error(err)
	}

	// Listen for allocation updated event
	event = waitForEvent(t, testChainServiceA.EventFeed(), chainservice.AllocationUpdatedEvent{})
	_, ok = event.(chainservice.AllocationUpdatedEvent)
	testhelpers.Assert(t, ok, "Expected allocation updated event")

	// TODO: Update off chain states

	// Check assets are liquidated
	balanceNodeA, _ = infra.anvilChain.GetAccountBalance(tc.Participants[0].Address())
	balanceNodeB, _ = infra.anvilChain.GetAccountBalance(tc.Participants[1].Address())
	t.Log("Balance of Alice", balanceNodeA, "\nBalance of Bob", balanceNodeB)
	// Assert balance equals ledger channel deposit since no payment has been made
	testhelpers.Assert(t, balanceNodeA.Cmp(big.NewInt(ledgerChannelDeposit)) == 0, "Balance of Alice (%v) should be equal to ledgerChannelDeposit (%v)", balanceNodeA, ledgerChannelDeposit)
	testhelpers.Assert(t, balanceNodeB.Cmp(big.NewInt(ledgerChannelDeposit)) == 0, "Balance of Bob (%v) should be equal to ledgerChannelDeposit (%v)", balanceNodeB, ledgerChannelDeposit)
}

func TestCheckpoint(t *testing.T) {
	tc := TestCase{
		Description:       "Checkpoint test",
		Chain:             AnvilChain,
		MessageService:    TestMessageService,
		MessageDelay:      0,
		LogName:           "Checkpoint_test",
		ChallengeDuration: 5,
		Participants: []TestParticipant{
			{StoreType: MemStore, Actor: testactors.Alice},
			{StoreType: MemStore, Actor: testactors.Bob},
		},
	}

	dataFolder, cleanup := testhelpers.GenerateTempStoreFolder()
	defer cleanup()

	infra := setupSharedInfra(tc)
	defer infra.Close(t)

	// Create go-nitro nodes
	nodeA, _, _, storeA, chainServiceA := setupIntegrationNode(tc, tc.Participants[0], infra, []string{}, dataFolder)
	defer nodeA.Close()
	nodeB, _, _, storeB, chainServiceB := setupIntegrationNode(tc, tc.Participants[1], infra, []string{}, dataFolder)
	defer nodeB.Close()

	// Separate chain service to listen for events
	testChainServiceB := setupChainService(tc, tc.Participants[1], infra)
	defer testChainServiceB.Close()

	// Create ledger channel
	ledgerChannel := openLedgerChannel(t, nodeA, nodeB, types.Address{}, uint32(tc.ChallengeDuration))

	// Store current state
	oldState := getLatestSignedState(storeA, ledgerChannel)

	// Conduct virtual fund and virtual defund
	virtualOutcome := initialPaymentOutcome(*nodeA.Address, *nodeB.Address, common.BigToAddress(common.Big0))
	response, err := nodeA.CreatePaymentChannel([]common.Address{}, *nodeB.Address, uint32(tc.ChallengeDuration), virtualOutcome)
	if err != nil {
		t.Error(err)
	}
	waitForObjectives(t, nodeA, nodeB, []node.Node{}, []protocols.ObjectiveId{response.Id})
	virtualDefundResponse, err := nodeA.ClosePaymentChannel(response.ChannelId)
	if err != nil {
		t.Error(err)
	}
	waitForObjectives(t, nodeA, nodeB, []node.Node{}, []protocols.ObjectiveId{virtualDefundResponse})

	// Store current state after virtual fund and defund
	newState := getLatestSignedState(storeB, ledgerChannel)

	// Alice calls challenge method using old state
	sendChallengeTransaction(t, oldState, tc.Participants[0].PrivateKey, ledgerChannel, chainServiceA)

	// Bob listens for challenge registered event
	event := waitForEvent(t, testChainServiceB.EventFeed(), chainservice.ChallengeRegisteredEvent{})
	t.Log("Challenge registed event received", event)
	challengeRegisteredEvent, ok := event.(chainservice.ChallengeRegisteredEvent)
	testhelpers.Assert(t, ok, "Expected challenge registered event")
	latestBlock, _ := infra.anvilChain.GetLatestBlock()
	testhelpers.Assert(t, latestBlock.Header().Time < challengeRegisteredEvent.FinalizesAt.Uint64(), "Expected channel to not be finalized")

	// Bob calls checkpoint method using new state
	checkpointTx := protocols.NewCheckpointTransaction(ledgerChannel, newState, make([]state.SignedState, 0))
	err = chainServiceB.SendTransaction(checkpointTx)
	if err != nil {
		t.Error(err)
	}

	// Listen for challenge cleared event
	event = waitForEvent(t, testChainServiceB.EventFeed(), chainservice.ChallengeClearedEvent{})
	t.Log("Challenge cleared event received", event)
	challengeClearedEvent, ok := event.(chainservice.ChallengeClearedEvent)
	testhelpers.Assert(t, ok, "Expected challenge cleared event")
	testhelpers.Assert(t, challengeClearedEvent.ChannelID() == ledgerChannel, "Channel ID mismatch")

	time.Sleep(time.Duration(tc.ChallengeDuration) * time.Second)
	latestBlock, _ = infra.anvilChain.GetLatestBlock()
	testhelpers.Assert(t, challengeRegisteredEvent.FinalizesAt.Uint64() <= latestBlock.Header().Time, "Expected challenge duration to be completed")

	// Alice attempts to liquidate the asset after the challenge duration, but the attempt fails because the outcome has not been finalized
	transferTx := protocols.NewTransferAllTransaction(ledgerChannel, oldState)
	err = chainServiceA.SendTransaction(transferTx)
	testhelpers.Assert(t, err.Error() == "execution reverted: revert: Channel not finalized.", "Expected execution reverted error")
}

func TestCounterChallenge(t *testing.T) {
	const payAmount = 2000

	tc := TestCase{
		Description:       "Counter challenge test",
		Chain:             AnvilChain,
		MessageService:    TestMessageService,
		MessageDelay:      0,
		LogName:           "Counter_challenge_test",
		ChallengeDuration: 5,
		Participants: []TestParticipant{
			{StoreType: MemStore, Actor: testactors.Alice},
			{StoreType: MemStore, Actor: testactors.Bob},
		},
	}

	dataFolder, cleanup := testhelpers.GenerateTempStoreFolder()
	defer cleanup()

	infra := setupSharedInfra(tc)
	defer infra.Close(t)

	// Create go-nitro nodes
	nodeA, _, _, storeA, chainServiceA := setupIntegrationNode(tc, tc.Participants[0], infra, []string{}, dataFolder)
	defer nodeA.Close()
	nodeB, _, _, storeB, chainServiceB := setupIntegrationNode(tc, tc.Participants[1], infra, []string{}, dataFolder)
	defer nodeB.Close()

	// Seperate chain service to listen for events
	testChainServiceB := setupChainService(tc, tc.Participants[1], infra)
	defer testChainServiceB.Close()

	// Create ledger channel and check balance of node
	ledgerChannel := openLedgerChannel(t, nodeA, nodeB, types.Address{}, uint32(tc.ChallengeDuration))
	balanceNodeA, _ := infra.anvilChain.GetAccountBalance(tc.Participants[0].Address())
	balanceNodeB, _ := infra.anvilChain.GetAccountBalance(tc.Participants[1].Address())
	t.Log("Balance of Alice", balanceNodeA, "\nBalance of Bob", balanceNodeB)
	testhelpers.Assert(t, balanceNodeA.Int64() == 0, "Balance of Alice should be zero")
	testhelpers.Assert(t, balanceNodeB.Int64() == 0, "Balance of Bob should be zero")

	// Store current state
	oldState := getLatestSignedState(storeA, ledgerChannel)

	// Conduct virtual fund, make payment and virtual defund
	virtualOutcome := initialPaymentOutcome(*nodeA.Address, *nodeB.Address, common.BigToAddress(common.Big0))
	response, err := nodeA.CreatePaymentChannel([]common.Address{}, *nodeB.Address, uint32(tc.ChallengeDuration), virtualOutcome)
	if err != nil {
		t.Error(err)
	}
	waitForObjectives(t, nodeA, nodeB, []node.Node{}, []protocols.ObjectiveId{response.Id})
	// Alice pays Bob
	nodeA.Pay(response.ChannelId, big.NewInt(payAmount))
	nodeBVoucher := <-nodeB.ReceivedVouchers()
	t.Logf("Voucher recieved %+v", nodeBVoucher)
	virtualDefundResponse, err := nodeA.ClosePaymentChannel(response.ChannelId)
	if err != nil {
		t.Error(err)
	}
	waitForObjectives(t, nodeA, nodeB, []node.Node{}, []protocols.ObjectiveId{virtualDefundResponse})

	// Store current state after payment and virtual defund
	newState := getLatestSignedState(storeB, ledgerChannel)

	// Alice calls challenge method using old state
	sendChallengeTransaction(t, oldState, tc.Participants[0].PrivateKey, ledgerChannel, chainServiceA)

	// Bob listens for challenge registered event
	event := waitForEvent(t, testChainServiceB.EventFeed(), chainservice.ChallengeRegisteredEvent{})
	t.Log("Challenge registed event received", event)
	challengeRegisteredEvent, ok := event.(chainservice.ChallengeRegisteredEvent)
	testhelpers.Assert(t, ok, "Expected challenge registered event")

	latestBlock, _ := infra.anvilChain.GetLatestBlock()
	testhelpers.Assert(t, latestBlock.Header().Time < challengeRegisteredEvent.FinalizesAt.Uint64(), "Expected channel to not be finalized")

	// Bob calls challenge method using new state
	sendChallengeTransaction(t, newState, tc.Participants[1].PrivateKey, ledgerChannel, chainServiceB)

	// Listen for challenge register event
	event = waitForEvent(t, testChainServiceB.EventFeed(), chainservice.ChallengeRegisteredEvent{})
	t.Log("Challenge registed event received", event)
	challengeRegisteredEvent, ok = event.(chainservice.ChallengeRegisteredEvent)
	testhelpers.Assert(t, ok, "Expected challenge registered event")

	time.Sleep(time.Duration(tc.ChallengeDuration) * time.Second)
	latestBlock, _ = infra.anvilChain.GetLatestBlock()
	testhelpers.Assert(t, challengeRegisteredEvent.FinalizesAt.Uint64() <= latestBlock.Header().Time, "Expected channel to be finalized")

	// Alice attempts to liquidate an asset with an outdated state but fails
	transferTx := protocols.NewTransferAllTransaction(ledgerChannel, oldState)
	err = chainServiceB.SendTransaction(transferTx)
	testhelpers.Assert(t, err.Error() == "execution reverted: revert: incorrect fingerprint", "Expected execution reverted error")

	// Bob calls transferAllAssets method using new state
	transferTx = protocols.NewTransferAllTransaction(ledgerChannel, newState)
	err = chainServiceB.SendTransaction(transferTx)
	if err != nil {
		t.Error(err)
	}

	// Listen for allocation updated event
	event = waitForEvent(t, testChainServiceB.EventFeed(), chainservice.AllocationUpdatedEvent{})
	_, ok = event.(chainservice.AllocationUpdatedEvent)
	testhelpers.Assert(t, ok, "Expected allocation updated event")

	// Check assets are liquidated
	balanceNodeA, _ = infra.anvilChain.GetAccountBalance(tc.Participants[0].Address())
	balanceNodeB, _ = infra.anvilChain.GetAccountBalance(tc.Participants[1].Address())
	t.Log("Balance of Alice", balanceNodeA, "\nBalance of Bob", balanceNodeB)
	// Alice's balance is determined by subtracting amount paid from her ledger deposit, while Bob's balance is calculated by adding his ledger deposit to the amount received
	testhelpers.Assert(t, balanceNodeA.Cmp(big.NewInt(ledgerChannelDeposit-payAmount)) == 0, "Balance of Alice  (%v) should be equal to (%v)", balanceNodeA, ledgerChannelDeposit-payAmount)
	testhelpers.Assert(t, balanceNodeB.Cmp(big.NewInt(ledgerChannelDeposit+payAmount)) == 0, "Balance of Bob (%v) should be equal to (%v)", balanceNodeB, ledgerChannelDeposit+payAmount)
}

func TestVirtualPaymentChannel(t *testing.T) {
	const payAmount = 2000

	tc := TestCase{
		Description:       "Virtual channel test",
		Chain:             AnvilChain,
		MessageService:    TestMessageService,
		MessageDelay:      0,
		LogName:           "Virtual_channel_test",
		ChallengeDuration: 5,
		Participants: []TestParticipant{
			{StoreType: MemStore, Actor: testactors.Alice},
			{StoreType: MemStore, Actor: testactors.Bob},
		},
	}

	dataFolder, cleanup := testhelpers.GenerateTempStoreFolder()
	defer cleanup()

	infra := setupSharedInfra(tc)
	defer infra.Close(t)

	// Create go-nitro nodes
	nodeA, _, _, storeA, chainServiceA := setupIntegrationNode(tc, tc.Participants[0], infra, []string{}, dataFolder)
	defer nodeA.Close()
	nodeB, _, _, storeB, _ := setupIntegrationNode(tc, tc.Participants[1], infra, []string{}, dataFolder)

	// Seperate chain service to listen for events
	testChainServiceB := setupChainService(tc, tc.Participants[1], infra)
	defer testChainServiceB.Close()

	// Create ledger channel
	ledgerChannel := openLedgerChannel(t, nodeA, nodeB, types.Address{}, uint32(tc.ChallengeDuration))

	// Create virtual channel
	virtualOutcome := initialPaymentOutcome(*nodeA.Address, *nodeB.Address, types.Address{})
	virtualResponse, _ := nodeA.CreatePaymentChannel([]common.Address{}, *nodeB.Address, uint32(tc.ChallengeDuration), virtualOutcome)

	// Wait for objective to complete
	waitForObjectives(t, nodeA, nodeB, []node.Node{}, []protocols.ObjectiveId{virtualResponse.Id})
	checkPaymentChannel(t, virtualResponse.ChannelId, virtualOutcome, query.Open, nodeA, nodeB)

	// Alice pays Bob
	nodeA.Pay(virtualResponse.ChannelId, big.NewInt(payAmount))

	// Wait for Bob to recieve voucher
	nodeBVoucher := <-nodeB.ReceivedVouchers()
	t.Logf("Voucher recieved %+v", nodeBVoucher)

	virtualChannel, _ := storeB.GetChannelById(virtualResponse.ChannelId)
	voucherState, _ := virtualChannel.LatestSignedState()

	voucherAmountSignatureData := VoucherAmountSignature{
		Amount:    nodeBVoucher.Amount,
		Signature: NitroAdjudicator.ConvertSignature(nodeBVoucher.Signature),
	}

	// Encode voucher amount and voucher signature
	voucherAmountSigTy, _ := abi.NewType("tuple", "", []abi.ArgumentMarshaling{
		{Name: "amount", Type: "uint256"},
		{Name: "signature", Type: "tuple", Components: []abi.ArgumentMarshaling{
			{Name: "v", Type: "uint8"},
			{Name: "r", Type: "bytes32"},
			{Name: "s", Type: "bytes32"},
		}},
	})

	arguments := abi.Arguments{
		{Type: voucherAmountSigTy},
	}

	dataEncoded, err := arguments.Pack(voucherAmountSignatureData)
	if err != nil {
		t.Fatalf("Failed to encode data: %v", err)
	}

	// Create expected payment outcome
	finalVirtualOutcome := finalPaymentOutcome(*nodeA.Address, *nodeB.Address, common.Address{}, 1, uint(nodeBVoucher.Amount.Int64()))

	// Construct variable part with updted outcome and app data
	vp := state.VariablePart{Outcome: finalVirtualOutcome, TurnNum: voucherState.State().TurnNum + 1, AppData: dataEncoded, IsFinal: voucherState.State().IsFinal}

	// Update state with constructed variable part
	newState := state.StateFromFixedAndVariablePart(voucherState.State().FixedPart(), vp)

	// Bob signs constructed state
	_, _ = virtualChannel.SignAndAddState(newState, &tc.Participants[1].PrivateKey)

	// Update virtual channel with updated state
	_ = storeA.SetChannel(virtualChannel)
	_ = storeB.SetChannel(virtualChannel)

	// Close Bob's node
	closeNode(t, &nodeB)

	signedLedgerState := getLatestSignedState(storeA, ledgerChannel)
	signedVirtualState, signedPostFundState := getVirtualSignedState(storeA, virtualResponse.ChannelId)

	// Alice calls challenge method on virtual channel
	virtualChallengerSig, _ := NitroAdjudicator.SignChallengeMessage(signedVirtualState.State(), tc.Participants[0].PrivateKey)
	virtualChallengeTx := protocols.NewChallengeTransaction(virtualResponse.ChannelId, signedVirtualState, []state.SignedState{signedPostFundState}, virtualChallengerSig)
	err = chainServiceA.SendTransaction(virtualChallengeTx)
	if err != nil {
		t.Error(err)
	}

	// Bob listens for challenge registered event
	event := waitForEvent(t, testChainServiceB.EventFeed(), chainservice.ChallengeRegisteredEvent{})
	t.Log("Challenge registed event received", event)
	challengeRegisteredEvent, ok := event.(chainservice.ChallengeRegisteredEvent)
	testhelpers.Assert(t, ok, "Expected challenge registered event")

	time.Sleep(time.Duration(tc.ChallengeDuration) * time.Second)
	latestBlock, _ := infra.anvilChain.GetLatestBlock()
	testhelpers.Assert(t, challengeRegisteredEvent.FinalizesAt.Uint64() <= latestBlock.Header().Time, "Expected channel to be finalized")

	// Alice calls challenge method on ledger channel
	challengerSig, _ := NitroAdjudicator.SignChallengeMessage(signedLedgerState.State(), tc.Participants[0].PrivateKey)
	challengeTx := protocols.NewChallengeTransaction(ledgerChannel, signedLedgerState, make([]state.SignedState, 0), challengerSig)
	err = chainServiceA.SendTransaction(challengeTx)
	if err != nil {
		t.Error(err)
	}

	// Bob listens for challenge registered event
	event = waitForEvent(t, testChainServiceB.EventFeed(), chainservice.ChallengeRegisteredEvent{})
	t.Log("Challenge registed event received", event)
	challengeRegisteredEvent, ok = event.(chainservice.ChallengeRegisteredEvent)
	testhelpers.Assert(t, ok, "Expected challenge registered event")

	time.Sleep(time.Duration(tc.ChallengeDuration) * time.Second)
	latestBlock, _ = infra.anvilChain.GetLatestBlock()
	testhelpers.Assert(t, challengeRegisteredEvent.FinalizesAt.Uint64() <= latestBlock.Header().Time, "Expected channel to be finalized")

	// Call Reclaim method after finalizing ledger channel and virtual channel
	convertedLedgerFixedPart := NitroAdjudicator.ConvertFixedPart(signedLedgerState.State().FixedPart())
	convertedLedgerVariablePart := NitroAdjudicator.ConvertVariablePart(signedLedgerState.State().VariablePart())
	virtualStateHash, _ := signedVirtualState.State().Hash()
	sourceOutcome := signedLedgerState.State().Outcome
	sourceOb, _ := sourceOutcome.Encode()
	targetOutcome := signedVirtualState.State().Outcome
	targetOb, _ := targetOutcome.Encode()

	reclaimArgs := NitroAdjudicator.IMultiAssetHolderReclaimArgs{
		SourceChannelId:       ledgerChannel,
		FixedPart:             convertedLedgerFixedPart,
		VariablePart:          convertedLedgerVariablePart,
		SourceOutcomeBytes:    sourceOb,
		SourceAssetIndex:      common.Big0,
		IndexOfTargetInSource: common.Big2,
		TargetStateHash:       virtualStateHash,
		TargetOutcomeBytes:    targetOb,
		TargetAssetIndex:      common.Big0,
	}

	reclaimTx := protocols.NewReclaimTransaction(ledgerChannel, reclaimArgs)
	err = chainServiceA.SendTransaction(reclaimTx)
	if err != nil {
		t.Error(err)
	}

	time.Sleep(2 * time.Second)

	// Compute new state outcome allocations
	aliceOutcomeAllocationAmount := signedLedgerState.State().Outcome[0].Allocations[0].Amount
	bobOutcomeAllocationAmount := signedLedgerState.State().Outcome[0].Allocations[1].Amount

	aliceOutcomeAllocationAmount.Add(aliceOutcomeAllocationAmount, signedVirtualState.State().Outcome[0].Allocations[0].Amount)
	bobOutcomeAllocationAmount.Add(bobOutcomeAllocationAmount, signedVirtualState.State().Outcome[0].Allocations[1].Amount)

	// Get latest ledger channel state
	latestLedgerState := getLatestSignedState(storeA, ledgerChannel)
	latestState := latestLedgerState.State()

	// Update state with new state outcome allocations
	latestState.Outcome[0].Allocations = outcome.Allocations{
		{
			Destination:    latestLedgerState.State().Outcome[0].Allocations[0].Destination,
			Amount:         aliceOutcomeAllocationAmount,
			AllocationType: outcome.NormalAllocationType,
			Metadata:       latestLedgerState.State().Outcome[0].Allocations[0].Metadata,
		},
		{
			Destination:    latestLedgerState.State().Outcome[0].Allocations[1].Destination,
			Amount:         bobOutcomeAllocationAmount,
			AllocationType: outcome.NormalAllocationType,
			Metadata:       latestLedgerState.State().Outcome[0].Allocations[1].Metadata,
		},
	}

	signedConstructedState := state.NewSignedState(latestState)

	// Alice calls transferAllAssets method
	transferTx := protocols.NewTransferAllTransaction(ledgerChannel, signedConstructedState)
	err = chainServiceA.SendTransaction(transferTx)

	testhelpers.Assert(t, err == nil, "Expected assets liquidated")

	time.Sleep(2 * time.Second)

	// Check assets are liquidated
	balanceNodeA, _ := infra.anvilChain.GetAccountBalance(tc.Participants[0].Address())
	balanceNodeB, _ := infra.anvilChain.GetAccountBalance(tc.Participants[1].Address())
	t.Log("Balance of Alice", balanceNodeA, "\nBalance of Bob", balanceNodeB)

	// Alice's balance is determined by subtracting amount paid from her ledger deposit, while Bob's balance is calculated by adding his ledger deposit to the amount received
	testhelpers.Assert(t, balanceNodeA.Cmp(big.NewInt(ledgerChannelDeposit-payAmount)) == 0, "Balance of Alice (%v) should be equal to (%v)", balanceNodeA, ledgerChannelDeposit-payAmount)
	testhelpers.Assert(t, balanceNodeB.Cmp(big.NewInt(ledgerChannelDeposit+payAmount)) == 0, "Balance of Bob (%v) should be equal to (%v)", balanceNodeB, ledgerChannelDeposit+payAmount)
}

func sendChallengeTransaction(t *testing.T, signedState state.SignedState, privateKey []byte, ledgerChannel types.Destination, chainService chainservice.ChainService) {
	challengerSig, _ := NitroAdjudicator.SignChallengeMessage(signedState.State(), privateKey)
	challengeTx := protocols.NewChallengeTransaction(ledgerChannel, signedState, make([]state.SignedState, 0), challengerSig)
	err := chainService.SendTransaction(challengeTx)
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
	postFundState := virtualChannel.SignedPostFundState()
	return virtualSignedState, postFundState
}
