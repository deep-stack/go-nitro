package swap

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/statechannels/go-nitro/channel/state/outcome"
	"github.com/statechannels/go-nitro/types"
)

// TODO: Persist data using durable store
type SwapStore interface {
	SetSwapKeeper(channelId types.Destination, sk SwapKeeper) error
	GetSwapKeeper(channelId types.Destination) (sk *SwapKeeper, err error)
	RemoveSwapKeeper(channelId types.Destination) error
}

// TODO: Create in memory swap store
type SwapManager struct {
	store SwapStore
	me    common.Address
}

func NewSwapManager(me types.Address, store SwapStore) *SwapManager {
	return &SwapManager{store, me}
}

func (sm *SwapManager) Register(channelId types.Destination, participant []types.Address, myIndex uint, outcome outcome.Exit) error {
	if v, _ := sm.store.GetSwapKeeper(channelId); v != nil {
		return fmt.Errorf("swap keeper already registered")
	}

	swapKeeper := NewSwapKeeper(channelId, participant, myIndex, outcome)
	err := sm.store.SetSwapKeeper(channelId, swapKeeper)
	if err != nil {
		return fmt.Errorf("error storing swap keeper: %w", err)
	}

	return nil
}
