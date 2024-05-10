package challenge

import (
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"

	ta "github.com/statechannels/go-nitro/internal/testactors"
	"github.com/statechannels/go-nitro/node"
	"github.com/statechannels/go-nitro/node/engine"
	"github.com/statechannels/go-nitro/node/engine/chainservice"
	"github.com/statechannels/go-nitro/node/engine/messageservice"
	"github.com/statechannels/go-nitro/node/engine/store"
)

func TestChallenge(t *testing.T) {
	t.Log("Test challenge protocol")

	// Start the chain & deploy contract
	sim, bindings, ethAccounts, err := chainservice.SetupSimulatedBackend(2)
	defer closeSimulatedChain(t, sim)
	if err != nil {
		t.Fatal(err)
	}

	// Create go-nitro nodes
	nodeA := setupNode(t, sim, bindings, ethAccounts[0], ta.Alice.PrivateKey)
	defer closeNode(t, &nodeA)

	nodeB := setupNode(t, sim, bindings, ethAccounts[1], ta.Bob.PrivateKey)
	defer closeNode(t, &nodeB)

	// Create ledger channel

	// Close the node B

	// Node A call challenge method and wait for challenge duration

	// Node A call transfer method and check assets are liquidated
}

func closeSimulatedChain(t *testing.T, chain chainservice.SimulatedChain) {
	if err := chain.Close(); err != nil {
		t.Fatal(err)
	}
}

func setupNode(t *testing.T, sim chainservice.SimulatedChain, bindings chainservice.Bindings, txSigner *bind.TransactOpts, pk []byte) node.Node {
	chainService, err := chainservice.NewSimulatedBackendChainService(sim, bindings, txSigner)
	if err != nil {
		t.Fatal(err)
	}

	broker := messageservice.NewBroker()
	messageservice := messageservice.NewTestMessageService(ta.Alice.Address(), broker, 0)

	memstore := store.NewMemStore(pk)

	nitronode := node.New(
		messageservice,
		chainService,
		memstore,
		&engine.PermissivePolicy{})

	return nitronode
}

func closeNode(t *testing.T, node *node.Node) {
	err := node.Close()
	if err != nil {
		t.Fatal(err)
	}
}
