package node_test

import (
	"context"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/statechannels/go-nitro/channel/state/outcome"
	"github.com/statechannels/go-nitro/internal/testactors"
	td "github.com/statechannels/go-nitro/internal/testdata"
	"github.com/statechannels/go-nitro/internal/testhelpers"
	"github.com/statechannels/go-nitro/node"
	"github.com/statechannels/go-nitro/node/engine/chainservice"
	Token "github.com/statechannels/go-nitro/node/engine/chainservice/erc20"
	"github.com/statechannels/go-nitro/node/engine/messageservice"
	p2pms "github.com/statechannels/go-nitro/node/engine/messageservice/p2p-message-service"
	"github.com/statechannels/go-nitro/node/query"
	"github.com/statechannels/go-nitro/protocols"
	"github.com/statechannels/go-nitro/types"
)

func TestSimpleIntegrationScenario(t *testing.T) {
	simpleCase := TestCase{
		Description:    "Simple test",
		Chain:          MockChain,
		MessageService: TestMessageService,
		NumOfChannels:  1,
		MessageDelay:   0,
		LogName:        "simple_integration",
		NumOfHops:      1,
		NumOfPayments:  1,
		Participants: []TestParticipant{
			{StoreType: MemStore, Actor: testactors.Alice},
			{StoreType: MemStore, Actor: testactors.Bob},
			{StoreType: MemStore, Actor: testactors.Irene},
		},
	}

	RunIntegrationTestCase(simpleCase, t)
}

func TestComplexIntegrationScenario(t *testing.T) {
	complexCase := TestCase{
		Description:    "Complex test",
		Chain:          SimulatedChain,
		MessageService: P2PMessageService,
		NumOfChannels:  5,
		MessageDelay:   0,
		LogName:        "complex_integration",
		NumOfHops:      2,
		NumOfPayments:  5,
		Participants: []TestParticipant{
			{StoreType: DurableStore, Actor: testactors.Alice},
			{StoreType: DurableStore, Actor: testactors.Bob},
			{StoreType: DurableStore, Actor: testactors.Irene},
			{StoreType: DurableStore, Actor: testactors.Ivan},
		},
	}
	RunIntegrationTestCase(complexCase, t)
}

func TestSwapFund(t *testing.T) {
	testCase := TestCase{
		Description:       "Direct defund with Challenge",
		Chain:             AnvilChain,
		MessageService:    TestMessageService,
		ChallengeDuration: 5,
		MessageDelay:      0,
		LogName:           "challenge_test",
		Participants: []TestParticipant{
			{StoreType: MemStore, Actor: testactors.Alice},
			{StoreType: MemStore, Actor: testactors.Bob},
			{StoreType: MemStore, Actor: testactors.Irene},
		},
	}

	dataFolder, cleanup := testhelpers.GenerateTempStoreFolder()
	defer cleanup()

	infra := setupSharedInfra(testCase)
	defer infra.Close(t)

	// TokenBinding
	_, err := Token.NewToken(infra.anvilChain.ContractAddresses.TokenAddresses[0], infra.anvilChain.EthClient)
	if err != nil {
		t.Fatal(err)
	}

	// Create go-nitro nodes
	nodeA, _, _, _, _ := setupIntegrationNode(testCase, testCase.Participants[0], infra, []string{}, dataFolder)
	defer nodeA.Close()
	nodeB, _, _, _, _ := setupIntegrationNode(testCase, testCase.Participants[1], infra, []string{}, dataFolder)
	defer nodeB.Close()

	outcomeEth := CreateLedgerOutcome(*nodeA.Address, *nodeB.Address, ledgerChannelDeposit, ledgerChannelDeposit+10, common.Address{})
	outcomeCustomToken := CreateLedgerOutcome(*nodeA.Address, *nodeB.Address, ledgerChannelDeposit+20, ledgerChannelDeposit+30, infra.anvilChain.ContractAddresses.TokenAddresses[0])

	outcomeCustomToken2 := CreateLedgerOutcome(*nodeA.Address, *nodeB.Address, ledgerChannelDeposit+40, ledgerChannelDeposit+50, infra.anvilChain.ContractAddresses.TokenAddresses[1])

	multiAssetOutcome := append(outcomeEth, outcomeCustomToken...)
	multiAssetOutcome = append(multiAssetOutcome, outcomeCustomToken2...)

	// Create ledger channel
	ledgerResponse, err := nodeA.CreateLedgerChannel(*nodeB.Address, uint32(testCase.ChallengeDuration), multiAssetOutcome)
	if err != nil {
		t.Fatal("error creating ledger channel", err)
	}

	t.Log("Waiting for direct-fund objective to complete...")

	chA := nodeA.ObjectiveCompleteChan(ledgerResponse.Id)
	chB := nodeB.ObjectiveCompleteChan(ledgerResponse.Id)
	<-chA
	<-chB

	multiassetSwapChannelOutcome := outcome.Exit{
		outcome.SingleAssetExit{
			Asset: common.Address{},
			Allocations: outcome.Allocations{
				outcome.Allocation{
					Destination: types.AddressToDestination(*nodeA.Address),
					Amount:      big.NewInt(int64(1001)),
				},
				outcome.Allocation{
					Destination: types.AddressToDestination(*nodeB.Address),
					Amount:      big.NewInt(int64(1002)),
				},
			},
		},
		outcome.SingleAssetExit{
			Asset: infra.anvilChain.ContractAddresses.TokenAddresses[0],
			Allocations: outcome.Allocations{
				outcome.Allocation{
					Destination: types.AddressToDestination(*nodeA.Address),
					Amount:      big.NewInt(int64(501)),
				},
				outcome.Allocation{
					Destination: types.AddressToDestination(*nodeB.Address),
					Amount:      big.NewInt(int64(502)),
				},
			},
		},
		outcome.SingleAssetExit{
			Asset: infra.anvilChain.ContractAddresses.TokenAddresses[1],
			Allocations: outcome.Allocations{
				outcome.Allocation{
					Destination: types.AddressToDestination(*nodeA.Address),
					Amount:      big.NewInt(int64(601)),
				},
				outcome.Allocation{
					Destination: types.AddressToDestination(*nodeB.Address),
					Amount:      big.NewInt(int64(602)),
				},
			},
		},
	}

	swapChannelresponse, err := nodeA.CreateSwapChannel(
		nil,
		*nodeB.Address,
		0,
		multiassetSwapChannelOutcome,
	)
	if err != nil {
		t.Fatal(err)
	}

	chB = nodeB.ObjectiveCompleteChan(swapChannelresponse.Id)
	<-nodeA.ObjectiveCompleteChan(swapChannelresponse.Id)
	<-chB

	t.Log("Completed swap-fund objective")

	checkSwapChannel(t, swapChannelresponse.ChannelId, multiassetSwapChannelOutcome, query.Open, nodeA, nodeB)

	// Initiate swap from Bob
	response1, err := nodeB.SwapAssets(swapChannelresponse.ChannelId, common.Address{}, infra.anvilChain.ContractAddresses.TokenAddresses[0], big.NewInt(100), big.NewInt(200))
	if err != nil {
		t.Fatal(err)
	}

	// Wait for objective to wait for confirmation
	time.Sleep(3 * time.Second)

	pendingSwap1, err := nodeA.GetPendingSwapByChannelId(response1.ChannelId)
	if err != nil {
		t.Fatal(err)
	}

	// Accept the swap
	err = nodeA.ConfirmSwap(pendingSwap1.SwapId(), types.Accepted)
	if err != nil {
		t.Fatal(err)
	}

	<-nodeB.ObjectiveCompleteChan(response1.Id)

	// Check swap channel after accepting swap
	modifiedOutcome1 := modifyOutcomeWithSwap(multiassetSwapChannelOutcome, pendingSwap1, 1)

	checkSwapChannel(t, swapChannelresponse.ChannelId, modifiedOutcome1, query.Open, nodeA, nodeB)

	// Initiate swap from Alice
	response2, err := nodeA.SwapAssets(swapChannelresponse.ChannelId, common.Address{}, infra.anvilChain.ContractAddresses.TokenAddresses[0], big.NewInt(100), big.NewInt(200))
	if err != nil {
		t.Fatal(err)
	}

	// Wait for objective to wait for confirmation
	time.Sleep(3 * time.Second)

	pendingSwap2, err := nodeB.GetPendingSwapByChannelId(response2.ChannelId)
	if err != nil {
		t.Fatal(err)
	}

	// Accept the swap
	err = nodeB.ConfirmSwap(pendingSwap2.SwapId(), types.Accepted)
	if err != nil {
		t.Fatal(err)
	}

	<-nodeA.ObjectiveCompleteChan(response2.Id)

	// Check swap channel after accepting swap
	modifiedOutcome2 := modifyOutcomeWithSwap(modifiedOutcome1, pendingSwap2, 0)

	checkSwapChannel(t, swapChannelresponse.ChannelId, modifiedOutcome2, query.Open, nodeA, nodeB)

	// Initiate swap from Alice
	response3, err := nodeA.SwapAssets(swapChannelresponse.ChannelId, common.Address{}, infra.anvilChain.ContractAddresses.TokenAddresses[0], big.NewInt(100), big.NewInt(200))
	if err != nil {
		t.Fatal(err)
	}

	// Wait for objective to wait for confirmation
	time.Sleep(3 * time.Second)

	pendingSwap3, err := nodeB.GetPendingSwapByChannelId(response3.ChannelId)
	if err != nil {
		t.Fatal(err)
	}

	// Bob Rejects the swap
	err = nodeB.ConfirmSwap(pendingSwap3.SwapId(), types.Rejected)
	if err != nil {
		t.Fatal(err)
	}

	<-nodeA.ObjectiveCompleteChan(response3.Id)

	checkSwapChannel(t, swapChannelresponse.ChannelId, modifiedOutcome2, query.Open, nodeA, nodeB)

	// Initiate another swap from Alice
	response4, err := nodeA.SwapAssets(swapChannelresponse.ChannelId, common.Address{}, infra.anvilChain.ContractAddresses.TokenAddresses[0], big.NewInt(100), big.NewInt(200))
	if err != nil {
		t.Fatal(err)
	}

	// Wait for objective to wait for confirmation
	time.Sleep(3 * time.Second)

	pendingSwap4, err := nodeB.GetPendingSwapByChannelId(response4.ChannelId)
	if err != nil {
		t.Fatal(err)
	}

	// Accept the swap
	err = nodeB.ConfirmSwap(pendingSwap4.SwapId(), types.Accepted)
	if err != nil {
		t.Fatal(err)
	}

	<-nodeA.ObjectiveCompleteChan(response4.Id)

	// Check swap channel after accepting swap
	modifiedOutcome4 := modifyOutcomeWithSwap(modifiedOutcome2, pendingSwap3, 0)
	checkSwapChannel(t, swapChannelresponse.ChannelId, modifiedOutcome4, query.Open, nodeA, nodeB)

	ledgerStateBeforeSdf, err := nodeA.GetSignedState(ledgerResponse.ChannelId)
	if err != nil {
		t.Fatal(err)
	}

	// Close crated swap channel
	res, err := nodeA.CloseSwapChannel(swapChannelresponse.ChannelId)
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Started swap-defund objective", "objectiveId", res)

	// Wait for swap-defund objectives to complete
	chA = nodeA.ObjectiveCompleteChan(res)
	chB = nodeB.ObjectiveCompleteChan(res)
	<-chA
	<-chB

	t.Log("Completed swap-defund objective")

	expectedLedgerOutcome := createExpectedLedgerOutcome(ledgerStateBeforeSdf.State().Outcome, modifiedOutcome4)
	checkLedgerChannel(t, ledgerResponse.ChannelId, expectedLedgerOutcome, query.Open, nodeA, nodeB)
}

// RunIntegrationTestCase runs the integration test case.
func RunIntegrationTestCase(tc TestCase, t *testing.T) {
	dataFolder, cleanup := testhelpers.GenerateTempStoreFolder()
	defer cleanup()

	t.Run(tc.Description, func(t *testing.T) {
		err := tc.Validate()
		if err != nil {
			t.Fatal(err)
		}
		infra := setupSharedInfra(tc)
		defer infra.Close(t)

		msgServices := make([]messageservice.MessageService, 0)

		// Setup clients
		t.Log("Initalizing intermediary node(s)...")
		intermediaries := make([]node.Node, 0)
		bootPeers := make([]string, 0)
		for _, intermediary := range tc.Participants[2:] {
			clientI, msgI, multiAddr, _, _ := setupIntegrationNode(tc, intermediary, infra, []string{}, dataFolder)

			intermediaries = append(intermediaries, clientI)
			msgServices = append(msgServices, msgI)
			bootPeers = append(bootPeers, multiAddr)
		}

		defer func() {
			for i := range intermediaries {
				intermediaries[i].Close()
			}
		}()
		t.Log("Intermediary node(s) setup complete")

		clientA, msgA, _, _, _ := setupIntegrationNode(tc, tc.Participants[0], infra, bootPeers, dataFolder)
		defer clientA.Close()
		msgServices = append(msgServices, msgA)

		clientB, msgB, _, _, _ := setupIntegrationNode(tc, tc.Participants[1], infra, bootPeers, dataFolder)
		defer clientB.Close()
		msgServices = append(msgServices, msgB)

		if tc.MessageService != TestMessageService {
			p2pServices := make([]*p2pms.P2PMessageService, len(tc.Participants))
			for i, msgService := range msgServices {
				p2pServices[i] = msgService.(*p2pms.P2PMessageService)
			}

			t.Log("Waiting for peer info exchange...")
			waitForPeerInfoExchange(p2pServices...)
			t.Log("Peer info exchange complete")
		}

		asset := common.Address{}
		// Setup ledger channels between Alice/Bob and intermediaries
		aliceLedgers := make([]types.Destination, tc.NumOfHops)
		bobLedgers := make([]types.Destination, tc.NumOfHops)
		for i, clientI := range intermediaries {
			t.Log("DEBUG: Setting up ledger channel between Alice/Bob and intermediaries, intermediary number", i)
			// Setup and check the ledger channel between Alice and the intermediary
			aliceLedgers[i] = openLedgerChannel(t, clientA, clientI, asset, 0)
			checkLedgerChannel(t, aliceLedgers[i], CreateLedgerOutcome(*clientA.Address, *clientI.Address, ledgerChannelDeposit, ledgerChannelDeposit, asset), query.Open, clientA)
			// Setup and check the ledger channel between Bob and the intermediary
			bobLedgers[i] = openLedgerChannel(t, clientI, clientB, asset, 0)
			checkLedgerChannel(t, bobLedgers[i], CreateLedgerOutcome(*clientI.Address, *clientB.Address, ledgerChannelDeposit, ledgerChannelDeposit, asset), query.Open, clientB)

		}

		t.Log("DEBUG: After setting up ledger channels between Alice/Bob and intermediaries")

		if tc.NumOfHops == 2 {
			openLedgerChannel(t, intermediaries[0], intermediaries[1], asset, 0)
			t.Log("DEBUG: After setting up ledger channels when NumOfHops is 2")
		}
		// Setup virtual channels
		objectiveIds := make([]protocols.ObjectiveId, tc.NumOfChannels)
		virtualIds := make([]types.Destination, tc.NumOfChannels)
		for i := 0; i < int(tc.NumOfChannels); i++ {
			outcome := td.Outcomes.Create(testactors.Alice.Address(), testactors.Bob.Address(), virtualChannelDeposit, 0, types.Address{})
			response, err := clientA.CreatePaymentChannel(
				clientAddresses(intermediaries),
				testactors.Bob.Address(),
				0,
				outcome,
			)
			if err != nil {
				t.Fatal(err)
			}

			t.Log("DEBUG: Created virtual channel, number: ", i)
			objectiveIds[i] = response.Id
			virtualIds[i] = response.ChannelId

		}
		// Wait for all the virtual channels to be ready
		waitForObjectives(t, clientA, clientB, intermediaries, objectiveIds)

		t.Log("DEBUG: After Setting up virtual channels")

		// Check all the virtual channels
		for i := 0; i < len(virtualIds); i++ {
			checkPaymentChannel(t,
				virtualIds[i],
				initialPaymentOutcome(*clientA.Address, *clientB.Address, asset),
				query.Open,
				clientA, clientB)
		}

		// Send payments
		for i := 0; i < len(virtualIds); i++ {
			for j := 0; j < int(tc.NumOfPayments); j++ {
				err = clientA.Pay(virtualIds[i], big.NewInt(int64(1)))
				if err != nil {
					t.Fatal(err)
				}
			}
		}

		t.Log("DEBUG: After making payments")

		// Wait for all the vouchers to be received by bob
		for i := 0; i < len(virtualIds)*int(tc.NumOfPayments); i++ {
			<-clientB.ReceivedVouchers()
		}

		t.Log("DEBUG: After waiting for vouchers")

		// Check the payment channels have the correct outcome after the payments
		for i := 0; i < len(virtualIds); i++ {
			checkPaymentChannel(t,
				virtualIds[i],
				finalPaymentOutcome(*clientA.Address, *clientB.Address, asset, tc.NumOfPayments, 1),
				query.Open,
				clientA, clientB)
		}

		t.Log("DEBUG: After checking payment channels")

		// Close virtual channels
		closeVirtualIds := make([]protocols.ObjectiveId, len(virtualIds))
		for i := 0; i < len(virtualIds); i++ {
			// alternative who is responsible for closing the channel
			switch i % 2 {
			case 0:
				closeVirtualIds[i], err = clientA.ClosePaymentChannel(virtualIds[i])
				if err != nil {
					t.Fatal(err)
				}
			case 1:
				closeVirtualIds[i], err = clientB.ClosePaymentChannel(virtualIds[i])
				if err != nil {
					t.Fatal(err)
				}
			}
		}

		waitForObjectives(t, clientA, clientB, intermediaries, closeVirtualIds)

		t.Log("DEBUG: After closing virtual channels")

		// Close all the ledger channels we opened

		closeLedgerChannel(t, clientA, intermediaries[0], aliceLedgers[0])
		checkLedgerChannel(t, aliceLedgers[0], finalAliceLedger(*intermediaries[0].Address, asset, tc.NumOfPayments, 1, tc.NumOfChannels), query.Complete, clientA)
		t.Log("DEBUG: After closing first alice ledger Channel")

		// TODO: This is brittle, we should generalize this to n number of intermediaries
		if tc.NumOfHops == 1 {
			closeLedgerChannel(t, intermediaries[0], clientB, bobLedgers[0])
			checkLedgerChannel(t, bobLedgers[0], finalBobLedger(*intermediaries[0].Address, asset, tc.NumOfPayments, 1, tc.NumOfChannels), query.Complete, clientB)
			t.Log("DEBUG: After closing ledger channel when NumOfHops is 1")
		}
		if tc.NumOfHops == 2 {
			closeLedgerChannel(t, intermediaries[1], clientB, bobLedgers[1])
			checkLedgerChannel(t, bobLedgers[1], finalBobLedger(*intermediaries[1].Address, asset, tc.NumOfPayments, 1, tc.NumOfChannels), query.Complete, clientB)
			t.Log("DEBUG: After closing ledger channel when NumOfHops is 2")
		}

		t.Log("DEBUG: After closing all ledger channels")

		var chainLastConfirmedBlockNum uint64
		if infra.mockChain != nil {
			chainLastConfirmedBlockNum = infra.mockChain.BlockNum
		} else if infra.simulatedChain != nil {
			latestBlock, err := infra.simulatedChain.BlockByNumber(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			chainLastConfirmedBlockNum = latestBlock.NumberU64() - chainservice.REQUIRED_BLOCK_CONFIRMATIONS
		}

		t.Log("DEBUG: Waiting for block confirmations")

		waitForClientBlockNum(t, clientA, chainLastConfirmedBlockNum, 10*time.Second)
		waitForClientBlockNum(t, clientB, chainLastConfirmedBlockNum, 10*time.Second)

		t.Log("DEBUG: After waiting for client block num")
	})
}

func waitForClientBlockNum(t *testing.T, n node.Node, targetBlockNum uint64, timeout time.Duration) {
	// Setup up a context with a timeout so we exit if we don't get the block num in time
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	lastBlockNum := uint64(0)
	var err error
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("expected block num of at least %d, got %d", targetBlockNum, lastBlockNum)
		default:
			lastBlockNum, err = n.GetLastBlockNum()
			if err != nil {
				t.Fatal(err)
			}
			if lastBlockNum >= targetBlockNum {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func TestSwapFundWithIntermediary(t *testing.T) {
	testCase := TestCase{
		Description:       "Direct defund with Challenge",
		Chain:             AnvilChain,
		MessageService:    P2PMessageService,
		ChallengeDuration: 5,
		MessageDelay:      0,
		LogName:           "challenge_test",
		Participants: []TestParticipant{
			{StoreType: MemStore, Actor: testactors.Alice},
			{StoreType: MemStore, Actor: testactors.Bob},
			{StoreType: MemStore, Actor: testactors.Irene},
		},
	}

	dataFolder, cleanup := testhelpers.GenerateTempStoreFolder()
	defer cleanup()

	infra := setupSharedInfra(testCase)
	defer infra.Close(t)

	// TokenBinding
	_, err := Token.NewToken(infra.anvilChain.ContractAddresses.TokenAddresses[0], infra.anvilChain.EthClient)
	if err != nil {
		t.Fatal(err)
	}

	// Create go-nitro nodes
	nodeB, _, nodeBMulitAddress, _, _ := setupIntegrationNode(testCase, testCase.Participants[1], infra, []string{}, dataFolder)
	defer nodeB.Close()
	nodeA, _, _, _, _ := setupIntegrationNode(testCase, testCase.Participants[0], infra, []string{nodeBMulitAddress}, dataFolder)
	defer nodeA.Close()
	nodeC, _, _, _, _ := setupIntegrationNode(testCase, testCase.Participants[2], infra, []string{nodeBMulitAddress}, dataFolder)
	defer nodeC.Close()

	// create 1st ledger channel
	outcomeEth := CreateLedgerOutcome(*nodeA.Address, *nodeB.Address, ledgerChannelDeposit, ledgerChannelDeposit, common.Address{})
	outcomeCustomToken := CreateLedgerOutcome(*nodeA.Address, *nodeB.Address, ledgerChannelDeposit, ledgerChannelDeposit, infra.anvilChain.ContractAddresses.TokenAddresses[0])
	outcomeCustomToken2 := CreateLedgerOutcome(*nodeA.Address, *nodeB.Address, ledgerChannelDeposit, ledgerChannelDeposit, infra.anvilChain.ContractAddresses.TokenAddresses[1])
	multiAssetOutcome := append(outcomeEth, outcomeCustomToken...)
	multiAssetOutcome = append(multiAssetOutcome, outcomeCustomToken2...)
	ledgerResponse, err := nodeA.CreateLedgerChannel(*nodeB.Address, uint32(testCase.ChallengeDuration), multiAssetOutcome)
	if err != nil {
		t.Fatal("error creating ledger channel", err)
	}
	t.Log("Waiting for direct-fund 1 objective to complete...")
	chA := nodeA.ObjectiveCompleteChan(ledgerResponse.Id)
	chB := nodeB.ObjectiveCompleteChan(ledgerResponse.Id)
	<-chA
	<-chB

	fmt.Println("LEDGER CHANNEL 1 created")
	time.Sleep(3 * time.Second)
	// create 2nd ledger channel
	outcomeEth2 := CreateLedgerOutcome(*nodeC.Address, *nodeB.Address, ledgerChannelDeposit, ledgerChannelDeposit, common.Address{})
	outcomeCustomToken3 := CreateLedgerOutcome(*nodeC.Address, *nodeB.Address, ledgerChannelDeposit, ledgerChannelDeposit, infra.anvilChain.ContractAddresses.TokenAddresses[0])
	outcomeCustomToken4 := CreateLedgerOutcome(*nodeC.Address, *nodeB.Address, ledgerChannelDeposit, ledgerChannelDeposit, infra.anvilChain.ContractAddresses.TokenAddresses[1])
	multiAssetOutcome2 := append(outcomeEth2, outcomeCustomToken3...)
	multiAssetOutcome2 = append(multiAssetOutcome2, outcomeCustomToken4...)
	ledgerResponse2, err := nodeC.CreateLedgerChannel(*nodeB.Address, uint32(testCase.ChallengeDuration), multiAssetOutcome2)
	if err != nil {
		t.Fatal("error creating ledger channel", err)
	}
	t.Log("Waiting for direct-fund 2 objective to complete...")
	chA2 := nodeB.ObjectiveCompleteChan(ledgerResponse2.Id)
	chB2 := nodeC.ObjectiveCompleteChan(ledgerResponse2.Id)
	<-chA2
	<-chB2

	fmt.Println("LEDGER CHANNEL 2 created")

	multiassetSwapChannelOutcome := outcome.Exit{
		outcome.SingleAssetExit{
			Asset: common.Address{},
			Allocations: outcome.Allocations{
				outcome.Allocation{
					Destination: types.AddressToDestination(*nodeA.Address),
					Amount:      big.NewInt(int64(1001)),
				},
				outcome.Allocation{
					Destination: types.AddressToDestination(*nodeC.Address),
					Amount:      big.NewInt(int64(1002)),
				},
			},
		},
		outcome.SingleAssetExit{
			Asset: infra.anvilChain.ContractAddresses.TokenAddresses[0],
			Allocations: outcome.Allocations{
				outcome.Allocation{
					Destination: types.AddressToDestination(*nodeA.Address),
					Amount:      big.NewInt(int64(501)),
				},
				outcome.Allocation{
					Destination: types.AddressToDestination(*nodeC.Address),
					Amount:      big.NewInt(int64(502)),
				},
			},
		},
		outcome.SingleAssetExit{
			Asset: infra.anvilChain.ContractAddresses.TokenAddresses[1],
			Allocations: outcome.Allocations{
				outcome.Allocation{
					Destination: types.AddressToDestination(*nodeA.Address),
					Amount:      big.NewInt(int64(601)),
				},
				outcome.Allocation{
					Destination: types.AddressToDestination(*nodeC.Address),
					Amount:      big.NewInt(int64(602)),
				},
			},
		},
	}

	swapChannelresponse, err := nodeA.CreateSwapChannel(
		[]common.Address{*nodeB.Address},
		*nodeC.Address,
		0,
		multiassetSwapChannelOutcome,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("Waiting for swap fund objective to complete")

	chB = nodeC.ObjectiveCompleteChan(swapChannelresponse.Id)
	<-nodeA.ObjectiveCompleteChan(swapChannelresponse.Id)
	<-chB

	t.Log("Completed swap-fund objective")

	// // Initiate swap from Alice
	// response1, err := nodeA.SwapAssets(swapChannelresponse.ChannelId, infra.anvilChain.ContractAddresses.TokenAddresses[0], infra.anvilChain.ContractAddresses.TokenAddresses[1], big.NewInt(100), big.NewInt(200))
	// if err != nil {
	// 	t.Fatal(err)
	// }

	// // Wait for objective to wait for confirmation
	// time.Sleep(2 * time.Second)

	// pendingSwap1, err := nodeC.GetPendingSwapByChannelId(swapChannelresponse.ChannelId)
	// if err != nil {
	// 	t.Fatal(err)
	// }

	// fmt.Println("PENDING SWAP", pendingSwap1)
	// // Accept the swap
	// err = nodeC.ConfirmSwap(pendingSwap1.SwapId(), types.Accepted)
	// if err != nil {
	// 	t.Fatal(err)
	// }

	// <-nodeA.ObjectiveCompleteChan(response1.Id)
	// fmt.Println("CONDUCT SWAP 1")
}
