package challenge

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"

	"github.com/statechannels/go-nitro/channel/state/outcome"
	ta "github.com/statechannels/go-nitro/internal/testactors"
	"github.com/statechannels/go-nitro/node"
	"github.com/statechannels/go-nitro/node/engine"
	"github.com/statechannels/go-nitro/node/engine/chainservice"
	"github.com/statechannels/go-nitro/node/engine/messageservice"
	"github.com/statechannels/go-nitro/node/engine/store"
	"github.com/statechannels/go-nitro/types"
)

func TestChallenge(t *testing.T) {
	t.Log("Test challenge protocol")

	// Start the chain & deploy contract
	sim, bindings, ethAccounts, err := chainservice.SetupSimulatedBackend(2)
	defer closeSimulatedChain(t, sim)
	if err != nil {
		t.Fatal(err)
	}

	broker := messageservice.NewBroker()

	// Create go-nitro nodes
	nodeA, messageServiceA := setupNode(t, sim, bindings, ethAccounts[0], ta.Alice.PrivateKey, broker, ta.Alice.Address())
	broker.Services[*nodeA.Address] = messageServiceA
	defer closeNode(t, &nodeA)

	nodeB, messageServiceB := setupNode(t, sim, bindings, ethAccounts[1], ta.Bob.PrivateKey, broker, ta.Bob.Address())
	broker.Services[*nodeB.Address] = messageServiceB
	// defer closeNode(t, &nodeB)

	// Create ledger channel
	ledgerChannel := CreateLedgerChannel(t, nodeA, nodeB)

	// Close the node B
	closeNode(t, &nodeB)

	// Node A call challenge method
	// 1. Get concluded signed state
	// 2. SignChallengeMessage
	// 3. Send transaction to chain
	// 4. Listen for event
	nodeA.ChallengeTransaction(ledgerChannel)

	// wait for challenge duration

	// Node A call transfer method and check assets are liquidated
}

func closeSimulatedChain(t *testing.T, chain chainservice.SimulatedChain) {
	if err := chain.Close(); err != nil {
		t.Fatal(err)
	}
}

func setupNode(t *testing.T, sim chainservice.SimulatedChain, bindings chainservice.Bindings, txSigner *bind.TransactOpts, pk []byte, broker messageservice.Broker, address common.Address) (node.Node, messageservice.TestMessageService) {
	chainService, err := chainservice.NewSimulatedBackendChainService(sim, bindings, txSigner)
	if err != nil {
		t.Fatal(err)
	}

	messageservice := messageservice.NewTestMessageService(address, broker, 0)

	memstore := store.NewMemStore(pk)

	nitronode := node.New(
		messageservice,
		chainService,
		memstore,
		&engine.PermissivePolicy{})

	return nitronode, messageservice
}

func closeNode(t *testing.T, node *node.Node) {
	err := node.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func createOutcome(first types.Address, second types.Address, x, y uint64, asset common.Address) outcome.Exit {
	return outcome.Exit{outcome.SingleAssetExit{
		Asset: asset,
		Allocations: outcome.Allocations{
			outcome.Allocation{
				Destination: types.AddressToDestination(first),
				Amount:      big.NewInt(int64(x)),
			},
			outcome.Allocation{
				Destination: types.AddressToDestination(second),
				Amount:      big.NewInt(int64(y)),
			},
		},
	}}
}

func CreateLedgerChannel(t *testing.T, nodeA node.Node, nodeB node.Node) types.Destination {
	outcome := createOutcome(*nodeA.Address, *nodeB.Address, 100000, 100000, types.Address{})

	response, err := nodeA.CreateLedgerChannel(*nodeB.Address, 5, outcome)
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Waiting for direct-fund objective to complete...")

	<-nodeA.ObjectiveCompleteChan(response.Id)
	<-nodeB.ObjectiveCompleteChan(response.Id)

	t.Log("Completed direct-fund objective", "channelId", response.ChannelId)

	return response.ChannelId
}
