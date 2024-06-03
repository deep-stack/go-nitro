package node_test

import (
	"math/big"
	"testing"
	"time"

	"github.com/statechannels/go-nitro/channel/state"
	"github.com/statechannels/go-nitro/internal/testactors"
	"github.com/statechannels/go-nitro/internal/testhelpers"
	"github.com/statechannels/go-nitro/node"
	"github.com/statechannels/go-nitro/node/query"
	"github.com/statechannels/go-nitro/protocols"
	"github.com/statechannels/go-nitro/types"
)

// Flow:
// 1. Create ledger channel on L1
// 2. Create mirrored channel on L2
// 3. Create virtual payment channel on top of mirrored ledger channel and make payments
// 4. Virtual defund and check balance on L2
func TestBridge(t *testing.T) {
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
