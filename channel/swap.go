package channel

import (
	"errors"
	"math"

	"github.com/statechannels/go-nitro/channel/state"
	"github.com/statechannels/go-nitro/types"
)

type SwapChannel struct {
	Channel
}

func NewSwapChannel(s state.State, myIndex uint) (*SwapChannel, error) {
	if int(myIndex) >= len(s.Participants) {
		return &SwapChannel{}, errors.New("myIndex not in range of the supplied participants")
	}

	for _, assetExit := range s.Outcome {
		if len(assetExit.Allocations) != 2 {
			return &SwapChannel{}, errors.New("a swap channel's initial state should only have two allocations")
		}
	}

	c, err := New(s, myIndex, types.Swap)

	return &SwapChannel{*c}, err
}

// Clone returns a pointer to a new, deep copy of the receiver, or a nil pointer if the receiver is nil.
func (v *SwapChannel) Clone() *SwapChannel {
	if v == nil {
		return nil
	}

	w := SwapChannel{*v.Channel.Clone()}

	return &w
}

// HasParticipantSignatures checks whether all major participants of the swap channel have signed the state
func (v *SwapChannel) HasParticipantSignatures(ss state.SignedState) bool {
	sigs := ss.Signatures()
	count := 0
	for _, sig := range sigs {
		if !sig.IsEmpty() {
			count++
		}
	}

	if count == 2 {
		return true
	}

	return false
}

// AddSignedSwapChannelState adds a signed swap channel state to the channel if all major participants of swap channel have signed it
func (v *SwapChannel) AddSignedSwapChannelState(ss state.SignedState) bool {
	if !v.HasParticipantSignatures(ss) {
		return false
	}

	if !v.Channel.AddSignedState(ss) {
		return false
	}

	v.Channel.OffChain.LatestSupportedSwapChannelStateTurnNum = ss.State().TurnNum
	return true
}

// LatestSupportedSwapChannelState fetches the lalest supported swap channel state
func (v *SwapChannel) LatestSupportedSwapChannelState() state.State {
	if v.Channel.OffChain.LatestSupportedStateTurnNum == MaxTurnNum {
		return state.State{}
	}

	if v.Channel.OffChain.LatestSupportedSwapChannelStateTurnNum == MaxTurnNum {
		return v.Channel.OffChain.SignedStateForTurnNum[v.Channel.OffChain.LatestSupportedStateTurnNum].State()
	}

	maxTurnNum := math.Max(float64(v.Channel.OffChain.LatestSupportedStateTurnNum), float64(v.Channel.OffChain.LatestSupportedSwapChannelStateTurnNum))

	ss := v.Channel.OffChain.SignedStateForTurnNum[uint64(maxTurnNum)]
	return ss.State()
}
