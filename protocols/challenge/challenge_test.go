package challenge

import (
	"testing"

	"github.com/statechannels/go-nitro/node/engine/chainservice"
)

func TestChallenge(t *testing.T) {
	t.Log("Test challenge protocol")

	// Start the chain & deploy contract
	sim, _, _, err := chainservice.SetupSimulatedBackend(2)
	defer closeSimulatedChain(t, sim)
	if err != nil {
		t.Fatal(err)
	}

	// Create two go-nitro node

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
