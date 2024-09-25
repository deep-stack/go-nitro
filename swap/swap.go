package swap

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/statechannels/go-nitro/channel/state"
	"github.com/statechannels/go-nitro/channel/state/outcome"
	"github.com/statechannels/go-nitro/types"
)

type Exchange struct {
	FromAsset  common.Address
	ToAsset    common.Address
	FromAmount *big.Int
	ToAmount   *big.Int
}

type Swap struct {
	channelId types.Destination
	exchange  Exchange
	sigs      map[uint]state.Signature // keyed by participant index in swap channel
}

func NewSwap(channelId types.Destination, fromAsset, toAsset common.Address, fromAmount, toAmout *big.Int) Swap {
	return Swap{
		channelId: channelId,
		exchange: Exchange{
			fromAsset,
			toAsset,
			fromAmount,
			toAmout,
		},
		sigs: make(map[uint]state.Signature, 2),
	}
}

type SwapKeeper struct {
	channelId      types.Destination
	participant    []types.Address
	MyIndex        uint
	channelOutcome outcome.Exit
	collector      []Swap
}

func NewSwapKeeper(channelId types.Destination, participant []types.Address, myIndex uint, outcome outcome.Exit) SwapKeeper {
	return SwapKeeper{
		channelId,
		participant,
		myIndex,
		outcome,
		make([]Swap, 0),
	}
}
