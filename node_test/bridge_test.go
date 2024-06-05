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
	"github.com/statechannels/go-nitro/internal/testhelpers"
	"github.com/statechannels/go-nitro/node"
	"github.com/statechannels/go-nitro/node/engine/chainservice"
	NitroAdjudicator "github.com/statechannels/go-nitro/node/engine/chainservice/adjudicator"
	"github.com/statechannels/go-nitro/node/query"
	"github.com/statechannels/go-nitro/payments"
	"github.com/statechannels/go-nitro/protocols"
	"github.com/statechannels/go-nitro/types"
)

func TestExitL2WithLedgerChannelState(t *testing.T) {
	tcL1 := TestCase{
		Chain:             AnvilChain,
		MessageService:    TestMessageService,
		MessageDelay:      0,
		LogName:           "Bridge_test",
		ChallengeDuration: 5,
		Participants: []TestParticipant{
			{StoreType: MemStore, Actor: testactors.Alice},
			{StoreType: MemStore, Actor: testactors.Bob},
		},
	}

	tcL2 := TestCase{
		Chain:             AnvilChain,
		MessageService:    TestMessageService,
		MessageDelay:      0,
		LogName:           "Bridge_test",
		ChallengeDuration: 5,
		Participants: []TestParticipant{
			{StoreType: MemStore, Actor: testactors.Bob},
			{StoreType: MemStore, Actor: testactors.Alice},
		},
		ChainPort: "8546",
	}

	dataFolder, cleanup := testhelpers.GenerateTempStoreFolder()
	defer cleanup()

	infraL1 := setupSharedInfra(tcL1)
	defer infraL1.Close(t)

	infraL2 := setupSharedInfra(tcL2)
	defer infraL2.Close(t)

	// Create go-nitro nodes
	nodeA, _, _, storeA, chainServiceA := setupIntegrationNode(tcL1, tcL1.Participants[0], infraL1, []string{}, dataFolder)
	defer nodeA.Close()

	nodeB, _, _, _, chainServiceB := setupIntegrationNode(tcL1, tcL1.Participants[1], infraL1, []string{}, dataFolder)
	defer nodeB.Close()

	nodeBPrime, _, _, storeBPrime, _ := setupIntegrationNode(tcL2, tcL2.Participants[0], infraL2, []string{}, dataFolder)
	defer nodeBPrime.Close()

	nodeAPrime, _, _, _, _ := setupIntegrationNode(tcL2, tcL2.Participants[1], infraL2, []string{}, dataFolder)
	defer nodeAPrime.Close()

	mirroredLedgerChannelId := types.Destination{}
	l1ChannelId := types.Destination{}

	t.Run("Create ledger channel on L1 and mirror it on L2", func(t *testing.T) {
		// Create ledger channel
		l1LedgerChannelId := openLedgerChannel(t, nodeA, nodeB, types.Address{}, uint32(tcL1.ChallengeDuration))
		l1ChannelId = l1LedgerChannelId

		l1LedgerChannel, err := storeA.GetConsensusChannelById(l1LedgerChannelId)
		if err != nil {
			t.Error(err)
		}

		l1ledgerChannelState := l1LedgerChannel.SupportedSignedState()
		l1ledgerChannelStateClone := l1ledgerChannelState.Clone()

		l1ledgerChannelStateClone.State().Outcome[0].Allocations[0].Destination = types.AddressToDestination(*nodeAPrime.Address)
		l1ledgerChannelStateClone.State().Outcome[0].Allocations[1].Destination = types.AddressToDestination(*nodeBPrime.Address)

		// Put NodeBPrime's allocation at index 0 as it creates mirrored ledger channel
		tempAllocation := l1ledgerChannelStateClone.State().Outcome[0].Allocations[0].Destination
		l1ledgerChannelStateClone.State().Outcome[0].Allocations[0].Destination = l1ledgerChannelStateClone.State().Outcome[0].Allocations[1].Destination
		l1ledgerChannelStateClone.State().Outcome[0].Allocations[1].Destination = tempAllocation

		// Create extended state outcome based on l1ChannelState
		l2ChannelOutcome := l1ledgerChannelStateClone.State().Outcome

		// Create mirrored ledger channel between node BPrime and APrime
		response, err := nodeBPrime.CreateBridgeChannel(*nodeAPrime.Address, uint32(tcL2.ChallengeDuration), l2ChannelOutcome)
		if err != nil {
			t.Error(err)
		}

		mirroredLedgerChannelId = response.ChannelId

		t.Log("Waiting for bridge-fund objective to complete...")

		<-nodeBPrime.ObjectiveCompleteChan(response.Id)
		<-nodeAPrime.ObjectiveCompleteChan(response.Id)

		t.Log("Completed bridge-fund objective")

		// Node B calls contract method to store L1ChannelId => L2ChannelId and L1ChannelId => L2ChannelId maps on contract
		genernateMirrorTx := protocols.NewGenerateMirrorTransaction(l1LedgerChannelId, mirroredLedgerChannelId)
		err = chainServiceB.SendTransaction(genernateMirrorTx)
		if err != nil {
			t.Error(err)
		}

		ledgerChannelInfo, err := nodeBPrime.GetLedgerChannel(mirroredLedgerChannelId)
		if err != nil {
			t.Error(err)
		}

		balanceNodeAPrime := ledgerChannelInfo.Balance.TheirBalance.ToInt()
		balanceNodeBPrime := ledgerChannelInfo.Balance.MyBalance.ToInt()
		t.Log("Balance of node BPrime", balanceNodeBPrime, "\nBalance of node APrime", balanceNodeAPrime)

		// BPrime's and APrime's balance should be equal to ledgerChannelDeposit since no payments happened
		testhelpers.Assert(t, balanceNodeBPrime.Cmp(big.NewInt(ledgerChannelDeposit)) == 0, "Balance of node BPrime (%v) should be equal to (%v)", balanceNodeBPrime, ledgerChannelDeposit)
		testhelpers.Assert(t, balanceNodeAPrime.Cmp(big.NewInt(ledgerChannelDeposit)) == 0, "Balance of node APrime (%v) should be equal to (%v)", balanceNodeAPrime, ledgerChannelDeposit)
	})

	t.Run("Exit to L1 using L2 ledger channel state", func(t *testing.T) {
		l2SignedState := getLatestSignedState(storeBPrime, mirroredLedgerChannelId)
		l2StateClone := l2SignedState.State().Clone()

		// Both participants on L2 ledger channel sign state where `isFinal = true` which is required for a channel to conclude and finalize
		l2StateClone.IsFinal = true

		Asig, _ := l2StateClone.Sign(tcL2.Participants[1].PrivateKey)
		Bsig, _ := l2StateClone.Sign(tcL2.Participants[0].PrivateKey)

		l2SignedStateClone := state.NewSignedState(l2StateClone)

		_ = l2SignedStateClone.AddSignature(Asig)
		_ = l2SignedStateClone.AddSignature(Bsig)

		// Node A calls modified `concludeAndTransferAllAssets` method to exit to L1 using L2 ledger channel state
		MirrorWithdrawAllTx := protocols.NewMirrorWithdrawAllTransaction(l1ChannelId, l2SignedStateClone)
		err := chainServiceA.SendTransaction(MirrorWithdrawAllTx)
		if err != nil {
			t.Error(err)
		}

		time.Sleep(2 * time.Second)

		balanceNodeA, _ := infraL1.anvilChain.GetAccountBalance(tcL1.Participants[0].Address())
		balanceNodeB, _ := infraL1.anvilChain.GetAccountBalance(tcL1.Participants[1].Address())
		t.Log("Balance of node A", balanceNodeA, "\nBalance of node B", balanceNodeB)

		// Node A's and node B's balance should be equal to ledgerChannelDeposit since no payments happened
		testhelpers.Assert(t, balanceNodeA.Cmp(big.NewInt(ledgerChannelDeposit)) == 0, "Balance of node A (%v) should be equal to (%v)", balanceNodeA, ledgerChannelDeposit)
		testhelpers.Assert(t, balanceNodeB.Cmp(big.NewInt(ledgerChannelDeposit)) == 0, "Balance of node B (%v) should be equal to (%v)", balanceNodeB, ledgerChannelDeposit)
	})
}

func TestExitL2WithPayments(t *testing.T) {
	const payAmount = 2000

	tcL1 := TestCase{
		Chain:             AnvilChain,
		MessageService:    TestMessageService,
		MessageDelay:      0,
		LogName:           "Bridge_test",
		ChallengeDuration: 5,
		Participants: []TestParticipant{
			{StoreType: MemStore, Actor: testactors.Alice},
			{StoreType: MemStore, Actor: testactors.Bob},
		},
	}

	tcL2 := TestCase{
		Chain:             AnvilChain,
		MessageService:    TestMessageService,
		MessageDelay:      0,
		LogName:           "Bridge_test",
		ChallengeDuration: 5,
		Participants: []TestParticipant{
			{StoreType: MemStore, Actor: testactors.Bob},
			{StoreType: MemStore, Actor: testactors.Alice},
		},
		ChainPort: "8546",
	}

	dataFolder, cleanup := testhelpers.GenerateTempStoreFolder()
	defer cleanup()

	infraL1 := setupSharedInfra(tcL1)
	defer infraL1.Close(t)

	infraL2 := setupSharedInfra(tcL2)
	defer infraL2.Close(t)

	// Create go-nitro nodes
	nodeA, _, _, storeA, chainServiceA := setupIntegrationNode(tcL1, tcL1.Participants[0], infraL1, []string{}, dataFolder)
	defer nodeA.Close()

	nodeB, _, _, _, chainServiceB := setupIntegrationNode(tcL1, tcL1.Participants[1], infraL1, []string{}, dataFolder)
	defer nodeB.Close()

	nodeBPrime, _, _, storeBPrime, _ := setupIntegrationNode(tcL2, tcL2.Participants[0], infraL2, []string{}, dataFolder)
	defer nodeBPrime.Close()

	nodeAPrime, _, _, _, _ := setupIntegrationNode(tcL2, tcL2.Participants[1], infraL2, []string{}, dataFolder)
	defer nodeAPrime.Close()

	mirroredLedgerChannelId := types.Destination{}
	l1ChannelId := types.Destination{}

	l2ChannelSignedState := state.SignedState{}

	t.Run("Create ledger channel on L1 and mirror it on L2", func(t *testing.T) {
		// Create ledger channel
		l1LedgerChannelId := openLedgerChannel(t, nodeA, nodeB, types.Address{}, uint32(tcL1.ChallengeDuration))
		l1ChannelId = l1LedgerChannelId

		l1LedgerChannel, err := storeA.GetConsensusChannelById(l1LedgerChannelId)
		if err != nil {
			t.Error(err)
		}

		l1ledgerChannelState := l1LedgerChannel.SupportedSignedState()
		l1ledgerChannelStateClone := l1ledgerChannelState.Clone()

		l1ledgerChannelStateClone.State().Outcome[0].Allocations[0].Destination = types.AddressToDestination(*nodeAPrime.Address)
		l1ledgerChannelStateClone.State().Outcome[0].Allocations[1].Destination = types.AddressToDestination(*nodeBPrime.Address)

		// Put NodeBPrime's allocation at index 0 as it creates mirrored ledger channel
		tempAllocation := l1ledgerChannelStateClone.State().Outcome[0].Allocations[0].Destination
		l1ledgerChannelStateClone.State().Outcome[0].Allocations[0].Destination = l1ledgerChannelStateClone.State().Outcome[0].Allocations[1].Destination
		l1ledgerChannelStateClone.State().Outcome[0].Allocations[1].Destination = tempAllocation

		// Create extended state outcome based on l1ChannelState
		l2ChannelOutcome := l1ledgerChannelStateClone.State().Outcome

		// Create mirrored ledger channel between node BPrime and APrime
		response, err := nodeBPrime.CreateBridgeChannel(*nodeAPrime.Address, uint32(tcL2.ChallengeDuration), l2ChannelOutcome)
		if err != nil {
			t.Error(err)
		}

		mirroredLedgerChannelId = response.ChannelId

		t.Log("Waiting for bridge-fund objective to complete...")

		<-nodeBPrime.ObjectiveCompleteChan(response.Id)
		<-nodeAPrime.ObjectiveCompleteChan(response.Id)

		t.Log("Completed bridge-fund objective")

		// Node B calls contract method to store L1ChannelId => L2ChannelId and L1ChannelId => L2ChannelId maps on contract
		genernateMirrorTx := protocols.NewGenerateMirrorTransaction(l1LedgerChannelId, mirroredLedgerChannelId)
		err = chainServiceB.SendTransaction(genernateMirrorTx)
		if err != nil {
			t.Error(err)
		}
	})

	t.Run("Create virtual channel on mirrored ledger channel and make payments", func(t *testing.T) {
		// Create virtual channel on mirrored ledger channel on L2 and make payments
		virtualOutcome := initialPaymentOutcome(*nodeBPrime.Address, *nodeAPrime.Address, types.Address{})

		virtualResponse, _ := nodeBPrime.CreatePaymentChannel([]types.Address{}, *nodeAPrime.Address, uint32(tcL2.ChallengeDuration), virtualOutcome)
		waitForObjectives(t, nodeBPrime, nodeAPrime, []node.Node{}, []protocols.ObjectiveId{virtualResponse.Id})

		checkPaymentChannel(t, virtualResponse.ChannelId, virtualOutcome, query.Open, nodeBPrime, nodeAPrime)

		virtualChannel, _ := storeBPrime.GetChannelById(virtualResponse.ChannelId)

		// Bridge pays APrime
		nodeBPrime.Pay(virtualResponse.ChannelId, big.NewInt(payAmount))

		// Wait for APrime to recieve voucher
		nodeAPrimeVoucher := <-nodeAPrime.ReceivedVouchers()
		t.Logf("Voucher recieved %+v", nodeAPrimeVoucher)

		// Virtual defund
		virtualDefundResponse, _ := nodeBPrime.ClosePaymentChannel(virtualChannel.Id)
		waitForObjectives(t, nodeBPrime, nodeAPrime, []node.Node{}, []protocols.ObjectiveId{virtualDefundResponse})

		ledgerChannelInfo, err := nodeBPrime.GetLedgerChannel(mirroredLedgerChannelId)
		if err != nil {
			t.Error(err)
		}
		balanceNodeAPrime := ledgerChannelInfo.Balance.TheirBalance.ToInt()
		balanceNodeBPrime := ledgerChannelInfo.Balance.MyBalance.ToInt()
		t.Log("Balance of node BPrime", balanceNodeBPrime, "\nBalance of node APrime", balanceNodeAPrime)

		l2SignedState := getLatestSignedState(storeBPrime, mirroredLedgerChannelId)
		l2StateClone := l2SignedState.State().Clone()

		// Both participants on L2 ledger channel sign state where `isFinal = true` which is required for a channel to conclude and finalize
		l2StateClone.IsFinal = true

		Asig, _ := l2StateClone.Sign(tcL2.Participants[1].PrivateKey)
		Bsig, _ := l2StateClone.Sign(tcL2.Participants[0].PrivateKey)

		l2SignedStateClone := state.NewSignedState(l2StateClone)

		_ = l2SignedStateClone.AddSignature(Asig)
		_ = l2SignedStateClone.AddSignature(Bsig)

		l2ChannelSignedState = l2SignedStateClone

		// BPrime's balance is determined by subtracting amount paid from it's ledger deposit, while APrime's balance is calculated by adding it's ledger deposit to the amount received
		testhelpers.Assert(t, balanceNodeBPrime.Cmp(big.NewInt(ledgerChannelDeposit-payAmount)) == 0, "Balance of node BPrime (%v) should be equal to (%v)", balanceNodeBPrime, ledgerChannelDeposit-payAmount)
		testhelpers.Assert(t, balanceNodeAPrime.Cmp(big.NewInt(ledgerChannelDeposit+payAmount)) == 0, "Balance of node APrime (%v) should be equal to (%v)", balanceNodeAPrime, ledgerChannelDeposit+payAmount)
	})

	t.Run("Exit to L1 using L2 ledger channel state", func(t *testing.T) {
		// Node A calls modified `concludeAndTransferAllAssets` method to exit to L1 using L2 ledger channel state
		MirrorWithdrawAllTx := protocols.NewMirrorWithdrawAllTransaction(l1ChannelId, l2ChannelSignedState)
		err := chainServiceA.SendTransaction(MirrorWithdrawAllTx)
		if err != nil {
			t.Error(err)
		}

		time.Sleep(2 * time.Second)

		balanceNodeA, _ := infraL1.anvilChain.GetAccountBalance(tcL1.Participants[0].Address())
		balanceNodeB, _ := infraL1.anvilChain.GetAccountBalance(tcL1.Participants[1].Address())
		t.Log("Balance of node A", balanceNodeA, "\nBalance of node B", balanceNodeB)

		testhelpers.Assert(t, balanceNodeA.Cmp(big.NewInt(ledgerChannelDeposit+payAmount)) == 0, "Balance of node A (%v) should be equal to (%v)", balanceNodeA, ledgerChannelDeposit+payAmount)
		testhelpers.Assert(t, balanceNodeB.Cmp(big.NewInt(ledgerChannelDeposit-payAmount)) == 0, "Balance of node B (%v) should be equal to (%v)", balanceNodeB, ledgerChannelDeposit-payAmount)
	})
}

func TestExitL2WithLedgerChannelStateUnilaterally(t *testing.T) {
	const payAmount = 2000

	tcL1 := TestCase{
		Chain:             AnvilChain,
		MessageService:    TestMessageService,
		MessageDelay:      0,
		LogName:           "Bridge_test",
		ChallengeDuration: 5,
		Participants: []TestParticipant{
			{StoreType: MemStore, Actor: testactors.Alice},
			{StoreType: MemStore, Actor: testactors.Bob},
		},
	}

	tcL2 := TestCase{
		Chain:             AnvilChain,
		MessageService:    TestMessageService,
		MessageDelay:      0,
		LogName:           "Bridge_test",
		ChallengeDuration: 5,
		Participants: []TestParticipant{
			{StoreType: MemStore, Actor: testactors.Bob},
			{StoreType: MemStore, Actor: testactors.Alice},
		},
		ChainPort: "8546",
	}

	dataFolder, cleanup := testhelpers.GenerateTempStoreFolder()
	defer cleanup()

	infraL1 := setupSharedInfra(tcL1)
	defer infraL1.Close(t)

	infraL2 := setupSharedInfra(tcL2)
	defer infraL2.Close(t)

	// Create go-nitro nodes
	nodeA, _, _, storeA, chainServiceA := setupIntegrationNode(tcL1, tcL1.Participants[0], infraL1, []string{}, dataFolder)
	defer nodeA.Close()

	nodeB, _, _, _, chainServiceB := setupIntegrationNode(tcL1, tcL1.Participants[1], infraL1, []string{}, dataFolder)

	nodeBPrime, _, _, storeBPrime, _ := setupIntegrationNode(tcL2, tcL2.Participants[0], infraL2, []string{}, dataFolder)

	nodeAPrime, _, _, storeAPrime, _ := setupIntegrationNode(tcL2, tcL2.Participants[1], infraL2, []string{}, dataFolder)
	defer nodeAPrime.Close()

	mirroredLedgerChannelId := types.Destination{}
	l1ChannelId := types.Destination{}

	// Separate chain service to listen for events
	testChainService := setupChainService(tcL1, tcL1.Participants[0], infraL1)
	defer testChainService.Close()

	l2ChannelSignedState := state.SignedState{}

	t.Run("Create ledger channel on L1 and mirror it on L2", func(t *testing.T) {
		// Create ledger channel
		l1LedgerChannelId := openLedgerChannel(t, nodeA, nodeB, types.Address{}, uint32(tcL1.ChallengeDuration))
		l1ChannelId = l1LedgerChannelId

		l1LedgerChannel, err := storeA.GetConsensusChannelById(l1LedgerChannelId)
		if err != nil {
			t.Error(err)
		}

		l1ledgerChannelState := l1LedgerChannel.SupportedSignedState()
		l1ledgerChannelStateClone := l1ledgerChannelState.Clone()

		l1ledgerChannelStateClone.State().Outcome[0].Allocations[0].Destination = types.AddressToDestination(*nodeAPrime.Address)
		l1ledgerChannelStateClone.State().Outcome[0].Allocations[1].Destination = types.AddressToDestination(*nodeBPrime.Address)

		// Put NodeBPrime's allocation at index 0 as it creates mirrored ledger channel
		tempAllocation := l1ledgerChannelStateClone.State().Outcome[0].Allocations[0].Destination
		l1ledgerChannelStateClone.State().Outcome[0].Allocations[0].Destination = l1ledgerChannelStateClone.State().Outcome[0].Allocations[1].Destination
		l1ledgerChannelStateClone.State().Outcome[0].Allocations[1].Destination = tempAllocation

		// Create extended state outcome based on l1ChannelState
		l2ChannelOutcome := l1ledgerChannelStateClone.State().Outcome

		// Create mirrored ledger channel between node BPrime and APrime
		response, err := nodeBPrime.CreateBridgeChannel(*nodeAPrime.Address, uint32(tcL2.ChallengeDuration), l2ChannelOutcome)
		if err != nil {
			t.Error(err)
		}

		mirroredLedgerChannelId = response.ChannelId

		t.Log("Waiting for bridge-fund objective to complete...")

		<-nodeBPrime.ObjectiveCompleteChan(response.Id)
		<-nodeAPrime.ObjectiveCompleteChan(response.Id)

		t.Log("Completed bridge-fund objective")

		// Node B calls contract method to store L1ChannelId => L2ChannelId and L1ChannelId => L2ChannelId maps on contract
		genernateMirrorTx := protocols.NewGenerateMirrorTransaction(l1LedgerChannelId, mirroredLedgerChannelId)
		err = chainServiceB.SendTransaction(genernateMirrorTx)
		if err != nil {
			t.Error(err)
		}
	})

	t.Run("Create virtual channel on mirrored ledger channel and make payments", func(t *testing.T) {
		// Create virtual channel on mirrored ledger channel on L2 and make payments
		virtualOutcome := initialPaymentOutcome(*nodeBPrime.Address, *nodeAPrime.Address, types.Address{})

		virtualResponse, _ := nodeBPrime.CreatePaymentChannel([]types.Address{}, *nodeAPrime.Address, uint32(tcL2.ChallengeDuration), virtualOutcome)
		waitForObjectives(t, nodeBPrime, nodeAPrime, []node.Node{}, []protocols.ObjectiveId{virtualResponse.Id})

		checkPaymentChannel(t, virtualResponse.ChannelId, virtualOutcome, query.Open, nodeBPrime, nodeAPrime)

		virtualChannel, _ := storeBPrime.GetChannelById(virtualResponse.ChannelId)

		// Bridge pays APrime
		nodeBPrime.Pay(virtualResponse.ChannelId, big.NewInt(payAmount))

		// Wait for APrime to recieve voucher
		nodeAPrimeVoucher := <-nodeAPrime.ReceivedVouchers()
		t.Logf("Voucher recieved %+v", nodeAPrimeVoucher)

		// Virtual defund
		virtualDefundResponse, _ := nodeBPrime.ClosePaymentChannel(virtualChannel.Id)
		waitForObjectives(t, nodeBPrime, nodeAPrime, []node.Node{}, []protocols.ObjectiveId{virtualDefundResponse})

		ledgerChannelInfo, err := nodeBPrime.GetLedgerChannel(mirroredLedgerChannelId)
		if err != nil {
			t.Error(err)
		}
		balanceNodeAPrime := ledgerChannelInfo.Balance.TheirBalance.ToInt()
		balanceNodeBPrime := ledgerChannelInfo.Balance.MyBalance.ToInt()
		t.Log("Balance of node BPrime", balanceNodeBPrime, "\nBalance of node APrime", balanceNodeAPrime)

		l2SignedState := getLatestSignedState(storeBPrime, mirroredLedgerChannelId)
		l2ChannelSignedState = l2SignedState

		// BPrime's balance is determined by subtracting amount paid from it's ledger deposit, while APrime's balance is calculated by adding it's ledger deposit to the amount received
		testhelpers.Assert(t, balanceNodeBPrime.Cmp(big.NewInt(ledgerChannelDeposit-payAmount)) == 0, "Balance of node BPrime (%v) should be equal to (%v)", balanceNodeBPrime, ledgerChannelDeposit-payAmount)
		testhelpers.Assert(t, balanceNodeAPrime.Cmp(big.NewInt(ledgerChannelDeposit+payAmount)) == 0, "Balance of node APrime (%v) should be equal to (%v)", balanceNodeAPrime, ledgerChannelDeposit+payAmount)
	})

	t.Run("Exit to L1 using L2 ledger channel state", func(t *testing.T) {
		// Close bridge nodes
		nodeB.Close()
		nodeBPrime.Close()

		// Node A calls modified `challenge` contract method with L2 ledger channel state
		challengerSig, _ := NitroAdjudicator.SignChallengeMessage(l2ChannelSignedState.State(), tcL1.Participants[0].PrivateKey)
		challengeTx := protocols.NewMirrorChallengeTransaction(l1ChannelId, l2ChannelSignedState, []state.SignedState{}, challengerSig)
		err := chainServiceA.SendTransaction(challengeTx)
		if err != nil {
			t.Error(err)
		}

		event := waitForEvent(t, testChainService.EventFeed(), chainservice.ChallengeRegisteredEvent{})
		t.Log("Challenge registed event received", event)
		challengeRegisteredEvent, ok := event.(chainservice.ChallengeRegisteredEvent)
		testhelpers.Assert(t, ok, "Expected challenge registered event")

		time.Sleep(time.Duration(tcL1.ChallengeDuration) * time.Second)
		latestBlock, _ := infraL1.anvilChain.GetLatestBlock()
		testhelpers.Assert(t, challengeRegisteredEvent.FinalizesAt.Uint64() <= latestBlock.Header().Time, "Expected channel to be finalized")

		l2SignedState := getLatestSignedState(storeAPrime, mirroredLedgerChannelId)

		mirrorTransferAllTx := protocols.NewMirrorTransferAllTransaction(l1ChannelId, l2SignedState)
		err = chainServiceA.SendTransaction(mirrorTransferAllTx)
		if err != nil {
			t.Error(err)
		}

		time.Sleep(2 * time.Second)

		balanceNodeA, _ := infraL1.anvilChain.GetAccountBalance(tcL1.Participants[0].Address())
		balanceNodeB, _ := infraL1.anvilChain.GetAccountBalance(tcL1.Participants[1].Address())
		t.Log("Balance of node A", balanceNodeA, "\nBalance of node B", balanceNodeB)

		// Node A's and node B's balance should be equal to ledgerChannelDeposit since no payments happened
		testhelpers.Assert(t, balanceNodeA.Cmp(big.NewInt(ledgerChannelDeposit+payAmount)) == 0, "Balance of node A (%v) should be equal to (%v)", balanceNodeA, ledgerChannelDeposit+payAmount)
		testhelpers.Assert(t, balanceNodeB.Cmp(big.NewInt(ledgerChannelDeposit-payAmount)) == 0, "Balance of node B (%v) should be equal to (%v)", balanceNodeB, ledgerChannelDeposit-payAmount)
	})
}

func TestExitL2WithVirtualChannelStateUnilaterally(t *testing.T) {
	const payAmount = 2000

	tcL1 := TestCase{
		Chain:             AnvilChain,
		MessageService:    TestMessageService,
		MessageDelay:      0,
		LogName:           "Bridge_test",
		ChallengeDuration: 5,
		Participants: []TestParticipant{
			{StoreType: MemStore, Actor: testactors.Alice},
			{StoreType: MemStore, Actor: testactors.Bob},
		},
	}

	tcL2 := TestCase{
		Chain:             AnvilChain,
		MessageService:    TestMessageService,
		MessageDelay:      0,
		LogName:           "Bridge_test",
		ChallengeDuration: 5,
		Participants: []TestParticipant{
			{StoreType: MemStore, Actor: testactors.Bob},
			{StoreType: MemStore, Actor: testactors.Alice},
		},
		ChainPort: "8546",
	}

	dataFolder, cleanup := testhelpers.GenerateTempStoreFolder()
	defer cleanup()

	infraL1 := setupSharedInfra(tcL1)
	defer infraL1.Close(t)

	infraL2 := setupSharedInfra(tcL2)
	defer infraL2.Close(t)

	// Create go-nitro nodes
	nodeA, _, _, storeA, chainServiceA := setupIntegrationNode(tcL1, tcL1.Participants[0], infraL1, []string{}, dataFolder)
	defer nodeA.Close()

	nodeB, _, _, _, chainServiceB := setupIntegrationNode(tcL1, tcL1.Participants[1], infraL1, []string{}, dataFolder)

	nodeBPrime, _, _, _, _ := setupIntegrationNode(tcL2, tcL2.Participants[0], infraL2, []string{}, dataFolder)

	nodeAPrime, _, _, storeAPrime, _ := setupIntegrationNode(tcL2, tcL2.Participants[1], infraL2, []string{}, dataFolder)
	defer nodeAPrime.Close()

	mirroredLedgerChannelId := types.Destination{}
	l1ChannelId := types.Destination{}

	// Separate chain service to listen for events
	testChainService := setupChainService(tcL1, tcL1.Participants[0], infraL1)
	defer testChainService.Close()

	virtualChannelId := types.Destination{}
	nodeAPrimeVirtualPaymentVoucher := payments.Voucher{}

	t.Run("Create ledger channel on L1 and mirror it on L2", func(t *testing.T) {
		// Create ledger channel
		l1LedgerChannelId := openLedgerChannel(t, nodeA, nodeB, types.Address{}, uint32(tcL1.ChallengeDuration))
		l1ChannelId = l1LedgerChannelId

		l1LedgerChannel, err := storeA.GetConsensusChannelById(l1LedgerChannelId)
		if err != nil {
			t.Error(err)
		}

		l1ledgerChannelState := l1LedgerChannel.SupportedSignedState()
		l1ledgerChannelStateClone := l1ledgerChannelState.Clone()

		l1ledgerChannelStateClone.State().Outcome[0].Allocations[0].Destination = types.AddressToDestination(*nodeAPrime.Address)
		l1ledgerChannelStateClone.State().Outcome[0].Allocations[1].Destination = types.AddressToDestination(*nodeBPrime.Address)

		// Put NodeBPrime's allocation at index 0 as it creates mirrored ledger channel
		tempAllocation := l1ledgerChannelStateClone.State().Outcome[0].Allocations[0].Destination
		l1ledgerChannelStateClone.State().Outcome[0].Allocations[0].Destination = l1ledgerChannelStateClone.State().Outcome[0].Allocations[1].Destination
		l1ledgerChannelStateClone.State().Outcome[0].Allocations[1].Destination = tempAllocation

		// Create extended state outcome based on l1ChannelState
		l2ChannelOutcome := l1ledgerChannelStateClone.State().Outcome

		// Create mirrored ledger channel between node BPrime and APrime
		response, err := nodeBPrime.CreateBridgeChannel(*nodeAPrime.Address, uint32(tcL2.ChallengeDuration), l2ChannelOutcome)
		if err != nil {
			t.Error(err)
		}

		mirroredLedgerChannelId = response.ChannelId

		t.Log("Waiting for bridge-fund objective to complete...")

		<-nodeBPrime.ObjectiveCompleteChan(response.Id)
		<-nodeAPrime.ObjectiveCompleteChan(response.Id)

		t.Log("Completed bridge-fund objective")

		// Node B calls contract method to store L1ChannelId => L2ChannelId and L1ChannelId => L2ChannelId maps on contract
		genernateMirrorTx := protocols.NewGenerateMirrorTransaction(l1LedgerChannelId, mirroredLedgerChannelId)
		err = chainServiceB.SendTransaction(genernateMirrorTx)
		if err != nil {
			t.Error(err)
		}
	})

	t.Run("Create virtual channel on mirrored ledger channel and make payments", func(t *testing.T) {
		// Create virtual channel on mirrored ledger channel on L2 and make payments
		virtualOutcome := initialPaymentOutcome(*nodeBPrime.Address, *nodeAPrime.Address, types.Address{})

		virtualResponse, _ := nodeBPrime.CreatePaymentChannel([]types.Address{}, *nodeAPrime.Address, uint32(tcL2.ChallengeDuration), virtualOutcome)
		waitForObjectives(t, nodeBPrime, nodeAPrime, []node.Node{}, []protocols.ObjectiveId{virtualResponse.Id})

		virtualChannelId = virtualResponse.ChannelId

		checkPaymentChannel(t, virtualResponse.ChannelId, virtualOutcome, query.Open, nodeBPrime, nodeAPrime)

		// Bridge pays APrime
		nodeBPrime.Pay(virtualResponse.ChannelId, big.NewInt(payAmount))

		// Close bridge nodes
		nodeB.Close()
		nodeBPrime.Close()

		// Wait for APrime to recieve voucher
		nodeAPrimeVoucher := <-nodeAPrime.ReceivedVouchers()
		t.Logf("Voucher recieved %+v", nodeAPrimeVoucher)

		nodeAPrimeVirtualPaymentVoucher = nodeAPrimeVoucher
	})

	t.Run("Exit to L1 using L2 ledger channel state", func(t *testing.T) {
		virtualChannel, _ := storeAPrime.GetChannelById(virtualChannelId)
		voucherState, _ := virtualChannel.LatestSignedState()

		// Create type to encode voucher amount and signature
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

		voucherAmountSignatureData := VoucherAmountSignature{
			Amount:    nodeAPrimeVirtualPaymentVoucher.Amount,
			Signature: NitroAdjudicator.ConvertSignature(nodeAPrimeVirtualPaymentVoucher.Signature),
		}

		// Use above created type and encode voucher amount and signature
		dataEncoded, err := arguments.Pack(voucherAmountSignatureData)
		if err != nil {
			t.Fatalf("Failed to encode data: %v", err)
		}

		// Create expected payment outcome
		finalVirtualOutcome := finalPaymentOutcome(*nodeA.Address, *nodeB.Address, common.Address{}, 1, uint(nodeAPrimeVirtualPaymentVoucher.Amount.Int64()))

		// Construct variable part with updated outcome and app data
		vp := state.VariablePart{Outcome: finalVirtualOutcome, TurnNum: voucherState.State().TurnNum + 1, AppData: dataEncoded, IsFinal: voucherState.State().IsFinal}

		// Update state with constructed variable part
		newState := state.StateFromFixedAndVariablePart(voucherState.State().FixedPart(), vp)

		// APrime signs constructed state and adds it to the virtual channel
		_, _ = virtualChannel.SignAndAddState(newState, &tcL2.Participants[1].PrivateKey)

		// Update store with updated virtual channel
		_ = storeAPrime.SetChannel(virtualChannel)

		// Get updated virtual channel
		updatedVirtualChannel, _ := storeAPrime.GetChannelById(virtualChannelId)
		signedVirtualState, _ := updatedVirtualChannel.LatestSignedState()
		signedPostFundState := updatedVirtualChannel.SignedPostFundState()

		// Node A calls modified `challenge` with L2 virtual channel state
		virtualChallengerSig, _ := NitroAdjudicator.SignChallengeMessage(signedVirtualState.State(), tcL1.Participants[0].PrivateKey)
		mirrroVirtualChallengeTx := protocols.NewMirrorChallengeTransaction(virtualChannelId, signedVirtualState, []state.SignedState{signedPostFundState}, virtualChallengerSig)
		err = chainServiceA.SendTransaction(mirrroVirtualChallengeTx)
		if err != nil {
			t.Error(err)
		}

		// Listen for challenge registered event
		event := waitForEvent(t, testChainService.EventFeed(), chainservice.ChallengeRegisteredEvent{})
		t.Log("Challenge registed event received", event)
		challengeRegisteredEvent, ok := event.(chainservice.ChallengeRegisteredEvent)
		testhelpers.Assert(t, ok, "Expected challenge registered event")

		time.Sleep(time.Duration(tcL2.ChallengeDuration) * time.Second)
		latestBlock, _ := infraL1.anvilChain.GetLatestBlock()
		testhelpers.Assert(t, challengeRegisteredEvent.FinalizesAt.Uint64() <= latestBlock.Header().Time, "Expected channel to be finalized")

		l2SignedState := getLatestSignedState(storeAPrime, mirroredLedgerChannelId)

		// Node A calls modified `challenge` with L2 ledger channel state
		challengerSig, _ := NitroAdjudicator.SignChallengeMessage(l2SignedState.State(), tcL1.Participants[0].PrivateKey)
		challengeTx := protocols.NewMirrorChallengeTransaction(l1ChannelId, l2SignedState, []state.SignedState{}, challengerSig)
		err = chainServiceA.SendTransaction(challengeTx)
		if err != nil {
			t.Error(err)
		}

		event = waitForEvent(t, testChainService.EventFeed(), chainservice.ChallengeRegisteredEvent{})
		t.Log("Challenge registed event received", event)
		challengeRegisteredEvent, ok = event.(chainservice.ChallengeRegisteredEvent)
		testhelpers.Assert(t, ok, "Expected challenge registered event")

		time.Sleep(time.Duration(tcL1.ChallengeDuration) * time.Second)
		latestBlock, _ = infraL1.anvilChain.GetLatestBlock()
		testhelpers.Assert(t, challengeRegisteredEvent.FinalizesAt.Uint64() <= latestBlock.Header().Time, "Expected channel to be finalized")

		l2SignedState = getLatestSignedState(storeAPrime, mirroredLedgerChannelId)
		updatedVirtualChannel, _ = storeAPrime.GetChannelById(virtualChannelId)
		signedVirtualState, _ = updatedVirtualChannel.LatestSignedState()

		// Now that ledger and virtual channels are finalized, call modified `reclaim` method
		convertedLedgerFixedPart := NitroAdjudicator.ConvertFixedPart(l2SignedState.State().FixedPart())
		convertedLedgerVariablePart := NitroAdjudicator.ConvertVariablePart(l2SignedState.State().VariablePart())
		virtualStateHash, _ := signedVirtualState.State().Hash()
		sourceOutcome := l2SignedState.State().Outcome
		sourceOb, _ := sourceOutcome.Encode()
		targetOutcome := signedVirtualState.State().Outcome
		targetOb, _ := targetOutcome.Encode()

		reclaimArgs := NitroAdjudicator.IMultiAssetHolderReclaimArgs{
			SourceChannelId:       mirroredLedgerChannelId,
			FixedPart:             convertedLedgerFixedPart,
			VariablePart:          convertedLedgerVariablePart,
			SourceOutcomeBytes:    sourceOb,
			SourceAssetIndex:      common.Big0,
			IndexOfTargetInSource: common.Big2,
			TargetStateHash:       virtualStateHash,
			TargetOutcomeBytes:    targetOb,
			TargetAssetIndex:      common.Big0,
		}

		reclaimTx := protocols.NewMirrorReclaimTransaction(l1ChannelId, reclaimArgs)
		err = chainServiceA.SendTransaction(reclaimTx)
		if err != nil {
			t.Error(err)
		}

		time.Sleep(2 * time.Second)
		l2SignedState = getLatestSignedState(storeAPrime, mirroredLedgerChannelId)

		// Compute new state outcome allocations
		aliceOutcomeAllocationAmount := l2SignedState.State().Outcome[0].Allocations[1].Amount
		bobOutcomeAllocationAmount := l2SignedState.State().Outcome[0].Allocations[0].Amount

		aliceOutcomeAllocationAmount.Add(aliceOutcomeAllocationAmount, signedVirtualState.State().Outcome[0].Allocations[1].Amount)
		bobOutcomeAllocationAmount.Add(bobOutcomeAllocationAmount, signedVirtualState.State().Outcome[0].Allocations[0].Amount)

		// Get latest ledger channel state
		latestState := l2SignedState.State()

		// Construct exit state with updated outcome allocations
		latestState.Outcome[0].Allocations = outcome.Allocations{
			{
				Destination:    l2SignedState.State().Outcome[0].Allocations[0].Destination,
				Amount:         bobOutcomeAllocationAmount,
				AllocationType: outcome.SimpleAllocationType,
				Metadata:       l2SignedState.State().Outcome[0].Allocations[0].Metadata,
			},
			{
				Destination:    l2SignedState.State().Outcome[0].Allocations[1].Destination,
				Amount:         aliceOutcomeAllocationAmount,
				AllocationType: outcome.SimpleAllocationType,
				Metadata:       l2SignedState.State().Outcome[0].Allocations[1].Metadata,
			},
		}

		signedConstructedState := state.NewSignedState(latestState)

		mirrorTransferAllTx := protocols.NewMirrorTransferAllTransaction(l1ChannelId, signedConstructedState)
		err = chainServiceA.SendTransaction(mirrorTransferAllTx)
		if err != nil {
			t.Error(err)
		}

		time.Sleep(2 * time.Second)

		balanceNodeA, _ := infraL1.anvilChain.GetAccountBalance(tcL1.Participants[0].Address())
		balanceNodeB, _ := infraL1.anvilChain.GetAccountBalance(tcL1.Participants[1].Address())
		t.Log("Balance of node A", balanceNodeA, "\nBalance of node B", balanceNodeB)

		testhelpers.Assert(t, balanceNodeA.Cmp(big.NewInt(ledgerChannelDeposit+payAmount)) == 0, "Balance of node A (%v) should be equal to (%v)", balanceNodeA, ledgerChannelDeposit+payAmount)
		testhelpers.Assert(t, balanceNodeB.Cmp(big.NewInt(ledgerChannelDeposit-payAmount)) == 0, "Balance of node B (%v) should be equal to (%v)", balanceNodeB, ledgerChannelDeposit-payAmount)
	})
}

func TestL2VirtualChannelWithIntermediary(t *testing.T) {
	const payAmount = 2000

	tcL1 := TestCase{
		Chain:             AnvilChain,
		MessageService:    TestMessageService,
		MessageDelay:      0,
		LogName:           "Bridge_test",
		ChallengeDuration: 5,
		Participants: []TestParticipant{
			{StoreType: MemStore, Actor: testactors.Alice},
			{StoreType: MemStore, Actor: testactors.Bob},
			{StoreType: MemStore, Actor: testactors.Irene},
		},
	}

	tcL2 := TestCase{
		Chain:             AnvilChain,
		MessageService:    TestMessageService,
		MessageDelay:      0,
		LogName:           "Bridge_test",
		ChallengeDuration: 5,
		Participants: []TestParticipant{
			{StoreType: MemStore, Actor: testactors.Bob},
			{StoreType: MemStore, Actor: testactors.Alice},
			{StoreType: MemStore, Actor: testactors.Irene},
		},
		ChainPort: "8546",
	}

	dataFolder, cleanup := testhelpers.GenerateTempStoreFolder()
	defer cleanup()

	infraL1 := setupSharedInfra(tcL1)
	defer infraL1.Close(t)

	infraL2 := setupSharedInfra(tcL2)
	defer infraL2.Close(t)

	// Create go-nitro nodes
	nodeA, _, _, storeA, _ := setupIntegrationNode(tcL1, tcL1.Participants[0], infraL1, []string{}, dataFolder)
	defer nodeA.Close()

	nodeB, _, _, _, _ := setupIntegrationNode(tcL1, tcL1.Participants[1], infraL1, []string{}, dataFolder)
	defer nodeB.Close()

	nodeC, _, _, storeC, _ := setupIntegrationNode(tcL1, tcL1.Participants[2], infraL1, []string{}, dataFolder)
	defer nodeC.Close()

	nodeBPrime, _, _, storeBPrime, _ := setupIntegrationNode(tcL2, tcL2.Participants[0], infraL2, []string{}, dataFolder)
	defer nodeBPrime.Close()

	nodeAPrime, _, _, _, _ := setupIntegrationNode(tcL2, tcL2.Participants[1], infraL2, []string{}, dataFolder)
	defer nodeAPrime.Close()

	nodeCPrime, _, _, _, _ := setupIntegrationNode(tcL2, tcL2.Participants[2], infraL2, []string{}, dataFolder)
	defer nodeCPrime.Close()

	mirroredLedgerChannel1Id := types.Destination{}
	mirroredLedgerChannel2Id := types.Destination{}

	t.Run("Create first ledger channel on L1 and mirror it on L2", func(t *testing.T) {
		// Create ledger channel
		l1LedgerChannel1Id := openLedgerChannel(t, nodeA, nodeB, types.Address{}, uint32(tcL1.ChallengeDuration))

		l1LedgerChannel1, err := storeA.GetConsensusChannelById(l1LedgerChannel1Id)
		if err != nil {
			t.Error(err)
		}

		l1ledgerChannel1State := l1LedgerChannel1.SupportedSignedState()
		l1ledgerChannel1StateClone := l1ledgerChannel1State.Clone()

		l1ledgerChannel1StateClone.State().Outcome[0].Allocations[0].Destination = types.AddressToDestination(*nodeAPrime.Address)
		l1ledgerChannel1StateClone.State().Outcome[0].Allocations[1].Destination = types.AddressToDestination(*nodeBPrime.Address)

		// Put NodeBPrime's allocation at index 0 as it creates mirrored ledger channel
		tempAllocation := l1ledgerChannel1StateClone.State().Outcome[0].Allocations[0].Destination
		l1ledgerChannel1StateClone.State().Outcome[0].Allocations[0].Destination = l1ledgerChannel1StateClone.State().Outcome[0].Allocations[1].Destination
		l1ledgerChannel1StateClone.State().Outcome[0].Allocations[1].Destination = tempAllocation

		// Create extended state outcome based on l1ChannelState
		l2Channel1Outcome := l1ledgerChannel1StateClone.State().Outcome

		// Create mirrored ledger channel between node BPrime and APrime
		response, err := nodeBPrime.CreateBridgeChannel(*nodeAPrime.Address, uint32(tcL2.ChallengeDuration), l2Channel1Outcome)
		if err != nil {
			t.Error(err)
		}

		mirroredLedgerChannel1Id = response.ChannelId

		t.Log("Waiting for bridge-fund objective to complete...")

		<-nodeBPrime.ObjectiveCompleteChan(response.Id)
		<-nodeAPrime.ObjectiveCompleteChan(response.Id)

		t.Log("Completed bridge-fund objective")
	})

	t.Run("Create second ledger channel on L1 and mirror it on L2", func(t *testing.T) {
		// Create ledger channel
		l1LedgerChannel2Id := openLedgerChannel(t, nodeC, nodeB, types.Address{}, uint32(tcL1.ChallengeDuration))

		l1LedgerChannel2, err := storeC.GetConsensusChannelById(l1LedgerChannel2Id)
		if err != nil {
			t.Error(err)
		}

		l1ledgerChannel2State := l1LedgerChannel2.SupportedSignedState()
		l1ledgerChannel2StateClone := l1ledgerChannel2State.Clone()

		l1ledgerChannel2StateClone.State().Outcome[0].Allocations[0].Destination = types.AddressToDestination(*nodeAPrime.Address)
		l1ledgerChannel2StateClone.State().Outcome[0].Allocations[1].Destination = types.AddressToDestination(*nodeBPrime.Address)

		// Put NodeBPrime's allocation at index 0 as it creates mirrored ledger channel
		tempAllocation := l1ledgerChannel2StateClone.State().Outcome[0].Allocations[0].Destination
		l1ledgerChannel2StateClone.State().Outcome[0].Allocations[0].Destination = l1ledgerChannel2StateClone.State().Outcome[0].Allocations[1].Destination
		l1ledgerChannel2StateClone.State().Outcome[0].Allocations[1].Destination = tempAllocation

		// Create extended state outcome based on l1ChannelState
		l2Channel2Outcome := l1ledgerChannel2StateClone.State().Outcome

		// Create mirrored ledger channel between node BPrime and APrime
		response, err := nodeBPrime.CreateBridgeChannel(*nodeCPrime.Address, uint32(tcL2.ChallengeDuration), l2Channel2Outcome)
		if err != nil {
			t.Error(err)
		}

		mirroredLedgerChannel2Id = response.ChannelId

		t.Log("Waiting for bridge-fund objective to complete...")

		<-nodeBPrime.ObjectiveCompleteChan(response.Id)
		<-nodeCPrime.ObjectiveCompleteChan(response.Id)

		t.Log("Completed bridge-fund objective")
	})

	t.Run("Create virtual channel between A' and C' and B' as intermediary and make payments", func(t *testing.T) {
		// Create virtual channel on mirrored ledger channel on L2 and make payments
		virtualOutcome := initialPaymentOutcome(*nodeAPrime.Address, *nodeCPrime.Address, types.Address{})

		virtualResponse, _ := nodeAPrime.CreatePaymentChannel([]types.Address{*nodeBPrime.Address}, *nodeCPrime.Address, uint32(tcL2.ChallengeDuration), virtualOutcome)
		waitForObjectives(t, nodeBPrime, nodeAPrime, []node.Node{}, []protocols.ObjectiveId{virtualResponse.Id})

		checkPaymentChannel(t, virtualResponse.ChannelId, virtualOutcome, query.Open, nodeAPrime, nodeBPrime, nodeCPrime)

		virtualChannel, _ := storeBPrime.GetChannelById(virtualResponse.ChannelId)

		// Bridge pays APrime
		nodeAPrime.Pay(virtualResponse.ChannelId, big.NewInt(payAmount))

		// Wait for APrime to recieve voucher
		nodeCPrimeVoucher := <-nodeCPrime.ReceivedVouchers()
		t.Logf("Voucher recieved %+v", nodeCPrimeVoucher)

		// Virtual defund
		virtualDefundResponse, _ := nodeAPrime.ClosePaymentChannel(virtualChannel.Id)
		waitForObjectives(t, nodeAPrime, nodeCPrime, []node.Node{nodeBPrime}, []protocols.ObjectiveId{virtualDefundResponse})

		ledgerChannel1Info, err := nodeBPrime.GetLedgerChannel(mirroredLedgerChannel1Id)
		if err != nil {
			t.Error(err)
		}

		ledgerChannel2Info, err := nodeBPrime.GetLedgerChannel(mirroredLedgerChannel2Id)
		if err != nil {
			t.Error(err)
		}

		balanceNodeAPrime := ledgerChannel1Info.Balance.TheirBalance.ToInt()
		balanceNodeBPrimeOnChannel1 := ledgerChannel1Info.Balance.MyBalance.ToInt()
		t.Log("Balance of node BPrime", balanceNodeBPrimeOnChannel1, "\nBalance of node APrime", balanceNodeAPrime)

		balanceNodeCPrime := ledgerChannel2Info.Balance.TheirBalance.ToInt()
		balanceNodeBPrimeOnChannel2 := ledgerChannel2Info.Balance.MyBalance.ToInt()
		t.Log("Balance of node BPrime", balanceNodeBPrimeOnChannel2, "\nBalance of node CPrime", balanceNodeCPrime)

		// On channel 1, APrime's balance is determined by subtracting payAmount from ledgerChannelDeposit and BPrime's balance is determined by adding payAmount to it's balance
		testhelpers.Assert(t, balanceNodeAPrime.Cmp(big.NewInt(ledgerChannelDeposit-payAmount)) == 0, "Balance of node APrime (%v) should be equal to (%v)", balanceNodeBPrimeOnChannel1, ledgerChannelDeposit-payAmount)
		testhelpers.Assert(t, balanceNodeBPrimeOnChannel1.Cmp(big.NewInt(ledgerChannelDeposit+payAmount)) == 0, "Balance of node BPrime on channel 1 (%v) should be equal to (%v)", balanceNodeAPrime, ledgerChannelDeposit+payAmount)

		// On channel 2, BPrime's balance is determined by subtracting payAmount from ledgerChannelDeposit and CPrime's balance is determined by adding payAmount to it's balance
		testhelpers.Assert(t, balanceNodeBPrimeOnChannel2.Cmp(big.NewInt(ledgerChannelDeposit-payAmount)) == 0, "Balance of node BPrime on channel 2 (%v) should be equal to (%v)", balanceNodeBPrimeOnChannel1, ledgerChannelDeposit-payAmount)
		testhelpers.Assert(t, balanceNodeCPrime.Cmp(big.NewInt(ledgerChannelDeposit+payAmount)) == 0, "Balance of node CPrime (%v) should be equal to (%v)", balanceNodeAPrime, ledgerChannelDeposit+payAmount)
	})
}
