package channel

import (
	"errors"

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

// HasSwapParticipantSignatures checks whether all end participants of the swap channel have signed the state
func (v *SwapChannel) HasSwapParticipantSignatures(ss state.SignedState) bool {
	sigs := ss.Signatures()
	count := 0

	if !sigs[0].IsEmpty() {
		count++
	}
	if !sigs[len(sigs)-1].IsEmpty() {
		count++
	}

	return count == 2
}

// AddSignedSwapChannelState adds a signed swap channel state to the channel if all major participants of swap channel have signed it
func (v *SwapChannel) AddSignedSwapChannelState(ss state.SignedState) bool {
	if !v.HasSwapParticipantSignatures(ss) {
		return false
	}

	return v.Channel.AddSignedState(ss)
}

// LatestSupportedSwapChannelState fetches the lalest supported swap channel state
func (v *SwapChannel) LatestSupportedSwapChannelState() state.State {
	latestTurn := v.Channel.OffChain.LatestSupportedStateTurnNum

	for turnNum, ss := range v.Channel.OffChain.SignedStateForTurnNum {
		if turnNum > latestTurn && v.HasSwapParticipantSignatures(ss) {
			latestTurn = turnNum
		}
	}

	ss := v.Channel.OffChain.SignedStateForTurnNum[latestTurn]
	return ss.State()
}
