package channel

import (
	"errors"
	"math"

	"github.com/statechannels/go-nitro/channel/state"
	"github.com/statechannels/go-nitro/types"
)

const PARTICIPANT_NODES_COUNT = 2

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

func (v *SwapChannel) HasParticipantSignatures(ss state.SignedState) bool {
	sigs := ss.Signatures()
	count := 0
	for _, sig := range sigs {
		// Count valid sigs
		if !sig.IsEmpty() {
			count++
		}
	}

	if count == PARTICIPANT_NODES_COUNT {
		return true
	}

	return false
}

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

func (v *SwapChannel) LatestSupportedSwapChannelState() state.State {
	supportedStateTurnNum := v.Channel.OffChain.LatestSupportedStateTurnNum
	supportedSwapChannelStateTurnNum := v.Channel.OffChain.LatestSupportedSwapChannelStateTurnNum
	maxTurnNum := math.Max(float64(supportedStateTurnNum), float64(supportedSwapChannelStateTurnNum))
	ss := v.Channel.OffChain.SignedStateForTurnNum[uint64(maxTurnNum)]
	return ss.State()
}
