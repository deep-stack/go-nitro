package channel

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/statechannels/go-nitro/channel/state"
	"github.com/statechannels/go-nitro/channel/state/outcome"
	"github.com/statechannels/go-nitro/node/engine/chainservice"
	"github.com/statechannels/go-nitro/types"
)

type OnChainData struct {
	Holdings                 types.Funds
	Outcome                  outcome.Exit
	StateHash                common.Hash
	FinalizesAt              *big.Int
	IsChallengeInitiatedByMe bool
	ChannelMode              ChannelMode
}

type OffChainData struct {
	SignedStateForTurnNum       map[uint64]state.SignedState // Longer term, we should have a more efficient and smart mechanism to store states https://github.com/statechannels/go-nitro/issues/106
	LatestSupportedStateTurnNum uint64                       // largest uint64 value reserved for "no supported state"
}

// Channel contains states and metadata and exposes convenience methods.
type Channel struct {
	state.FixedPart
	Id      types.Destination
	MyIndex uint
	Type    ChannelType

	OnChain  OnChainData
	OffChain OffChainData

	LastChainUpdate ChainUpdateData
}

type ChainUpdateData struct {
	BlockNum uint64
	TxIndex  uint
}

// isNewChainEvent returns true if the event has a greater block number (or equal blocknumber but with greater tx index) than prior chain events process by the receiver.
func (c *Channel) isNewChainEvent(event chainservice.Event) bool {
	return event.Block().BlockNum > c.LastChainUpdate.BlockNum ||
		(event.Block().BlockNum == c.LastChainUpdate.BlockNum && event.TxIndex() > c.LastChainUpdate.TxIndex)
}

// New constructs a new Channel from the supplied state.
func New(s state.State, myIndex uint, channelType ChannelType) (*Channel, error) {
	c := Channel{}
	var err error = s.Validate()

	if err != nil {
		return &c, err
	}

	c.Id = s.ChannelId()

	if err != nil {
		return &c, err
	}
	c.MyIndex = myIndex
	c.OnChain.Holdings = make(types.Funds)
	c.OnChain.FinalizesAt = big.NewInt(0)
	c.OnChain.IsChallengeInitiatedByMe = false
	c.OnChain.ChannelMode = Open
	c.FixedPart = s.FixedPart().Clone()
	c.OffChain.LatestSupportedStateTurnNum = MaxTurnNum // largest uint64 value reserved for "no supported state"

	// Store prefund
	c.OffChain.SignedStateForTurnNum = make(map[uint64]state.SignedState)
	c.OffChain.SignedStateForTurnNum[PreFundTurnNum] = state.NewSignedState(s)

	// Store postfund
	post := s.Clone()
	post.TurnNum = PostFundTurnNum
	c.OffChain.SignedStateForTurnNum[PostFundTurnNum] = state.NewSignedState(post)

	// Set on chain holdings to zero for each asset
	for asset := range s.Outcome.TotalAllocated() {
		c.OnChain.Holdings[asset] = big.NewInt(0)
	}

	c.Type = channelType
	return &c, nil
}

// jsonChannel replaces Channel's private fields with public ones,
// making it suitable for serialization
type jsonChannel struct {
	Id      types.Destination
	MyIndex uint
	state.FixedPart
	OnChain  OnChainData
	OffChain OffChainData
	Type     ChannelType
}

// MarshalJSON returns a JSON representation of the Channel
func (c Channel) MarshalJSON() ([]byte, error) {
	jsonCh := jsonChannel{
		Id:        c.Id,
		MyIndex:   c.MyIndex,
		OnChain:   c.OnChain,
		OffChain:  c.OffChain,
		FixedPart: c.FixedPart,
		Type:      c.Type,
	}
	return json.Marshal(jsonCh)
}

// UnmarshalJSON populates the calling Channel with the
// json-encoded data
func (c *Channel) UnmarshalJSON(data []byte) error {
	var jsonCh jsonChannel
	err := json.Unmarshal(data, &jsonCh)
	if err != nil {
		return fmt.Errorf("error unmarshaling channel data: %w", err)
	}

	c.Id = jsonCh.Id
	c.MyIndex = jsonCh.MyIndex
	c.OnChain = jsonCh.OnChain
	c.OffChain = jsonCh.OffChain

	c.FixedPart = jsonCh.FixedPart
	c.Type = jsonCh.Type

	return nil
}

// MyDestination returns the client's destination
func (c Channel) MyDestination() types.Destination {
	return types.AddressToDestination(c.Participants[c.MyIndex])
}

// Clone returns a pointer to a new, deep copy of the receiver, or a nil pointer if the receiver is nil.
func (c *Channel) Clone() *Channel {
	if c == nil {
		return nil
	}
	d, _ := New(c.PreFundState().Clone(), c.MyIndex, c.Type)
	d.OffChain.LatestSupportedStateTurnNum = c.OffChain.LatestSupportedStateTurnNum
	for i, ss := range c.OffChain.SignedStateForTurnNum {
		d.OffChain.SignedStateForTurnNum[i] = ss.Clone()
	}
	d.FixedPart = c.FixedPart.Clone()
	d.OnChain.Holdings = c.OnChain.Holdings
	d.OnChain.FinalizesAt = c.OnChain.FinalizesAt
	d.OnChain.ChannelMode = c.OnChain.ChannelMode
	d.OnChain.StateHash = c.OnChain.StateHash
	d.OnChain.IsChallengeInitiatedByMe = c.OnChain.IsChallengeInitiatedByMe
	return d
}

// PreFundState() returns the pre fund setup state for the channel.
func (c Channel) PreFundState() state.State {
	return c.OffChain.SignedStateForTurnNum[PreFundTurnNum].State()
}

// SignedPreFundState returns the signed pre fund setup state for the channel.
func (c Channel) SignedPreFundState() state.SignedState {
	return c.OffChain.SignedStateForTurnNum[PreFundTurnNum]
}

// PostFundState() returns the post fund setup state for the channel.
func (c Channel) PostFundState() state.State {
	return c.OffChain.SignedStateForTurnNum[PostFundTurnNum].State()
}

// SignedPostFundState() returns the SIGNED post fund setup state for the channel.
func (c Channel) SignedPostFundState() state.SignedState {
	return c.OffChain.SignedStateForTurnNum[PostFundTurnNum]
}

// PreFundSignedByMe returns true if the calling client has signed the pre fund setup state, false otherwise.
func (c Channel) PreFundSignedByMe() bool {
	if _, ok := c.OffChain.SignedStateForTurnNum[PreFundTurnNum]; ok {
		if c.OffChain.SignedStateForTurnNum[PreFundTurnNum].HasSignatureForParticipant(c.MyIndex) {
			return true
		}
	}
	return false
}

// PostFundSignedByMe returns true if the calling client has signed the post fund setup state, false otherwise.
func (c Channel) PostFundSignedByMe() bool {
	if _, ok := c.OffChain.SignedStateForTurnNum[PostFundTurnNum]; ok {
		if c.OffChain.SignedStateForTurnNum[PostFundTurnNum].HasSignatureForParticipant(c.MyIndex) {
			return true
		}
	}
	return false
}

// PreFundComplete() returns true if I have a complete set of signatures on  the pre fund setup state, false otherwise.
func (c Channel) PreFundComplete() bool {
	return c.OffChain.SignedStateForTurnNum[PreFundTurnNum].HasAllSignatures()
}

// PostFundComplete() returns true if I have a complete set of signatures on  the pre fund setup state, false otherwise.
func (c Channel) PostFundComplete() bool {
	return c.OffChain.SignedStateForTurnNum[PostFundTurnNum].HasAllSignatures()
}

// FinalSignedByMe returns true if the calling client has signed a final state, false otherwise.
func (c Channel) FinalSignedByMe() bool {
	for _, ss := range c.OffChain.SignedStateForTurnNum {
		if ss.HasSignatureForParticipant(c.MyIndex) && ss.State().IsFinal {
			return true
		}
	}
	return false
}

// FinalCompleted returns true if I have a complete set of signatures on a final state, false otherwise.
func (c Channel) FinalCompleted() bool {
	if c.OffChain.LatestSupportedStateTurnNum == MaxTurnNum {
		return false
	}

	return c.OffChain.SignedStateForTurnNum[c.OffChain.LatestSupportedStateTurnNum].State().IsFinal
}

// HasSupportedState returns true if the channel has a supported state, false otherwise.
func (c Channel) HasSupportedState() bool {
	return c.OffChain.LatestSupportedStateTurnNum != MaxTurnNum
}

// LatestSupportedState returns the latest supported state. A state is supported if it is signed
// by all participants.
func (c Channel) LatestSupportedState() (state.State, error) {
	signedState, err := c.LatestSupportedSignedState()
	if err != nil {
		return state.State{}, err
	}
	return signedState.State(), err
}

// LatestSupportedSignedState returns latest supported signed state. A state is supported if it is signed
// by all participants.
func (c Channel) LatestSupportedSignedState() (state.SignedState, error) {
	if c.OffChain.LatestSupportedStateTurnNum == MaxTurnNum {
		return state.SignedState{}, errors.New(`no state is yet supported`)
	}
	return c.OffChain.SignedStateForTurnNum[c.OffChain.LatestSupportedStateTurnNum], nil
}

// LatestSignedState fetches the state with the largest turn number signed by at least one participant.
func (c Channel) LatestSignedState() (state.SignedState, error) {
	if len(c.OffChain.SignedStateForTurnNum) == 0 {
		return state.SignedState{}, errors.New("no states are signed")
	}
	latestTurn := uint64(0)
	for k := range c.OffChain.SignedStateForTurnNum {
		if k > latestTurn {
			latestTurn = k
		}
	}
	return c.OffChain.SignedStateForTurnNum[latestTurn], nil
}

// Total() returns the total allocated of each asset allocated by the pre fund setup state of the Channel.
func (c Channel) Total() types.Funds {
	return c.PreFundState().Outcome.TotalAllocated()
}

// Affords returns true if, for each asset keying the input variables, the channel can afford the allocation given the funding.
// The decision is made based on the latest supported state of the channel.
//
// Both arguments are maps keyed by the same asset
func (c Channel) Affords(
	allocationMap map[common.Address]outcome.Allocation,
	fundingMap types.Funds,
) bool {
	lss, err := c.LatestSupportedState()
	if err != nil {
		return false
	}
	return lss.Outcome.Affords(allocationMap, fundingMap)
}

// AddStateWithSignature constructs a SignedState from the passed state and signature, and calls s.AddSignedState with it.
func (c *Channel) AddStateWithSignature(s state.State, sig state.Signature) bool {
	ss := state.NewSignedState(s)
	if err := ss.AddSignature(sig); err != nil {
		return false
	} else {
		return c.AddSignedState(ss)
	}
}

// AddSignedState adds a signed state to the Channel, updating the LatestSupportedState and Support if appropriate.
// Returns false and does not alter the channel if the state is "stale", belongs to a different channel, or is signed by a non participant.
func (c *Channel) AddSignedState(ss state.SignedState) bool {
	s := ss.State()

	if cId := s.ChannelId(); cId != c.Id {
		// Channel mismatch
		return false
	}

	if c.OffChain.LatestSupportedStateTurnNum != MaxTurnNum && s.TurnNum < c.OffChain.LatestSupportedStateTurnNum {
		// Stale state
		return false
	}

	// Store the signatures. If we have no record yet, add one.
	if signedState, ok := c.OffChain.SignedStateForTurnNum[s.TurnNum]; !ok {
		c.OffChain.SignedStateForTurnNum[s.TurnNum] = ss
	} else {
		err := signedState.Merge(ss)
		if err != nil {
			return false
		}
	}

	// Update latest supported state
	if c.OffChain.SignedStateForTurnNum[s.TurnNum].HasAllSignatures() {
		c.OffChain.LatestSupportedStateTurnNum = s.TurnNum
	}

	return true
}

// SignAndAddPrefund signs and adds the prefund state for the channel, returning a state.SignedState suitable for sending to peers.
func (c *Channel) SignAndAddPrefund(sk *[]byte) (state.SignedState, error) {
	return c.SignAndAddState(c.PreFundState(), sk)
}

// SignAndAddPrefund signs and adds the postfund state for the channel, returning a state.SignedState suitable for sending to peers.
func (c *Channel) SignAndAddPostfund(sk *[]byte) (state.SignedState, error) {
	return c.SignAndAddState(c.PostFundState(), sk)
}

// SignAndAddState signs and adds the state to the channel, returning a state.SignedState suitable for sending to peers.
func (c *Channel) SignAndAddState(s state.State, sk *[]byte) (state.SignedState, error) {
	sig, err := s.Sign(*sk)
	if err != nil {
		return state.SignedState{}, fmt.Errorf("could not sign prefund %w", err)
	}
	ss := state.NewSignedState(s)
	err = ss.AddSignature(sig)
	if err != nil {
		return state.SignedState{}, fmt.Errorf("could not add own signature %w", err)
	}
	ok := c.AddSignedState(ss)
	if !ok {
		return state.SignedState{}, fmt.Errorf("could not add signed state to channel %w", err)
	}
	return ss, nil
}

// UpdateWithChainEvent mutates the receiver with the supplied chain event, replacing the relevant data fields.
func (c *Channel) UpdateWithChainEvent(event chainservice.Event) (*Channel, error) {
	if !c.isNewChainEvent(event) {
		return nil, fmt.Errorf("chain event older than channel's last update")
	}
	// Process event
	switch e := event.(type) {
	case chainservice.AllocationUpdatedEvent:
		// TODO: Mark channel as complete(for unilateral exits), then channel finalized for (happy exits)
		c.OnChain.Holdings[e.AssetAddress] = e.AssetAmount
		// TODO: update OnChain.StateHash and OnChain.Outcome
	case chainservice.DepositedEvent:
		c.OnChain.Holdings[e.Asset] = e.NowHeld
	case chainservice.ConcludedEvent:
		break // TODO: update OnChain.StateHash and OnChain.Outcome
	case chainservice.ChallengeRegisteredEvent:
		h, err := e.StateHash(c.FixedPart)
		if err != nil {
			return nil, err
		}
		c.OnChain.StateHash = h
		c.OnChain.Outcome = e.Outcome()
		c.OnChain.FinalizesAt = e.FinalizesAt
		c.OnChain.IsChallengeInitiatedByMe = e.IsInitiatedByMe

		// Add a signed state from a chain event to a channel only if the channel ID matches
		// If challenge is registered on the L2 state then then channel Ids will be different
		if c.Id == event.ChannelID() {
			ss, err := e.SignedState(c.FixedPart)
			if err != nil {
				return nil, err
			}

			c.AddSignedState(ss)
		}

	case chainservice.ChallengeClearedEvent:
		// On chain, statusOf map is updated with the same values below following a checkpoint transaction in ForceMove contract
		c.OnChain.StateHash = common.Hash{}
		c.OnChain.Outcome = outcome.Exit{}
		c.OnChain.FinalizesAt = common.Big0

	// TODO: Handle Checkpointed event
	// Checkpointed event is emitted after a checkpoint transaction occurs on an open channel
	// checkpoint method of ForceMove.sol contract
	case chainservice.ReclaimedEvent:
	// TODO: Handle ReclaimedEvent
	case chainservice.StatusUpdatedEvent:
		c.OnChain.StateHash = e.StateHash
	default:
		return &Channel{}, fmt.Errorf("channel %+v cannot handle event %+v", c, event)
	}

	// Update Channel.LastChainUpdate
	c.LastChainUpdate.BlockNum = event.Block().BlockNum
	c.LastChainUpdate.TxIndex = event.TxIndex()
	return c, nil
}

// UpdateChannelMode update channel mode based on the channel FinalizesAt timestamp and latest block timestamp
func (c *Channel) UpdateChannelMode(latestBlockTime uint64) {
	if c.OnChain.FinalizesAt.Cmp(big.NewInt(0)) == 0 {
		c.OnChain.ChannelMode = Open
	} else if c.OnChain.FinalizesAt.Cmp(new(big.Int).SetUint64(latestBlockTime)) <= 0 {
		c.OnChain.ChannelMode = Finalized
	} else {
		c.OnChain.ChannelMode = Challenge
	}
}
