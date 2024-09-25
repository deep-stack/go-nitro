package swap

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/statechannels/go-nitro/channel"
	"github.com/statechannels/go-nitro/channel/state"
	"github.com/statechannels/go-nitro/protocols"
	"github.com/statechannels/go-nitro/types"
)

type GetChannelByIdFunction func(id types.Destination) (channel *channel.Channel, ok bool)

type Exchange struct {
	FromAsset  common.Address
	ToAsset    common.Address
	FromAmount *big.Int
	ToAmount   *big.Int
}

type SwapPrimitive struct {
	ChannelId types.Destination
	Exchange  Exchange
	Sigs      map[uint]state.Signature // keyed by participant index in swap channel
}

func NewSwapPrimitive(channelId types.Destination, fromAsset, toAsset common.Address, fromAmount, toAmout *big.Int) SwapPrimitive {
	return SwapPrimitive{
		ChannelId: channelId,
		Exchange: Exchange{
			fromAsset,
			toAsset,
			fromAmount,
			toAmout,
		},
		Sigs: make(map[uint]state.Signature, 2),
	}
}

const (
	WaitingForSwapPrimitive protocols.WaitingFor = "WaitingForSwapPrimitive"
	WaitingForNothing       protocols.WaitingFor = "WaitingForNothing" // Finished
)

const (
	SwapPrimitivePayload protocols.PayloadType = "SwapPrimitivePayload"
)

const ObjectivePrefix = "Swap-"

// Objective is a cache of data computed by reading from the store. It stores (potentially) infinite data.
type Objective struct {
	Status        protocols.ObjectiveStatus
	C             *channel.SwapChannel
	SwapPrimitive SwapPrimitive
	IsSwapper     bool
}

// NewObjective creates a new swap objective from a given request.
func NewObjective(request ObjectiveRequest, preApprove bool, isSwapper bool, getChannelFunc GetChannelByIdFunction) (Objective, error) {
	obj := Objective{}

	swapChannel, ok := getChannelFunc(request.ChannelId)
	if !ok {
		return obj, fmt.Errorf("new swap objective creation failed, swap channel not found")
	}

	if preApprove {
		obj.Status = protocols.Approved
	} else {
		obj.Status = protocols.Unapproved
	}

	if isSwapper {
		swapPrimitive := NewSwapPrimitive(request.ChannelId, request.FromAsset, request.ToAsset, request.FromAmount, request.ToAmount)
		obj.SwapPrimitive = swapPrimitive
		obj.IsSwapper = true
	}

	obj.C = &channel.SwapChannel{
		Channel: *swapChannel,
	}

	return obj, nil
}

// Id returns the objective id.
func (o *Objective) Id() protocols.ObjectiveId {
	// TODO: Determine objective id
	// TODO: Each objective id should be unique for each swap primitive
	return protocols.ObjectiveId(ObjectivePrefix + o.C.Id.String())
}

// Approve returns an approved copy of the objective.
func (o *Objective) Approve() protocols.Objective {
	updated := o.clone()
	// todo: consider case of s.Status == Rejected
	updated.Status = protocols.Approved

	return &updated
}

// Reject returns a rejected copy of the objective.
func (o *Objective) Reject() (protocols.Objective, protocols.SideEffects) {
	updated := o.clone()
	updated.Status = protocols.Rejected

	peer := o.C.Participants[1-o.C.MyIndex]
	messages := protocols.CreateRejectionNoticeMessage(o.Id(), peer)
	sideEffects := protocols.SideEffects{MessagesToSend: messages}
	return &updated, sideEffects
}

// OwnsChannel returns the channel that the objective is funding.
func (o *Objective) OwnsChannel() types.Destination {
	return o.C.Id
}

// GetStatus returns the status of the objective.
func (o *Objective) GetStatus() protocols.ObjectiveStatus {
	return o.Status
}

// Update receives an protocols.ObjectiveEvent, applies all applicable event data to the VirtualFundObjective,
// and returns the updated state.
func (o *Objective) Update(raw protocols.ObjectivePayload) (protocols.Objective, error) {
	updated := o.clone()
	return &updated, nil
}

// Crank inspects the extended state and declares a list of Effects to be executed
// It's like a state machine transition function where the finite / enumerable state is returned (computed from the extended state)
// rather than being independent of the extended state; and where there is only one type of event ("the crank") with no data on it at all.
func (o *Objective) Crank(secretKey *[]byte) (protocols.Objective, protocols.SideEffects, protocols.WaitingFor, error) {
	updated := o.clone()

	sideEffects := protocols.SideEffects{}
	// Input validation
	if updated.Status != protocols.Approved {
		return &updated, sideEffects, WaitingForNothing, protocols.ErrNotApproved
	}

	// Completion
	updated.Status = protocols.Completed
	return &updated, sideEffects, WaitingForNothing, nil
}

func (o *Objective) Related() []protocols.Storable {
	ret := []protocols.Storable{o.C}

	return ret
}

//////////////////////////////////////////////////
//  Private methods on the Swap Objective //
//////////////////////////////////////////////////

// Clone returns a deep copy of the receiver.
func (o *Objective) clone() Objective {
	clone := Objective{}
	clone.Status = o.Status

	return clone
}

func (o Objective) MarshalJSON() ([]byte, error) {
	// TODO: create marshal method
	return json.Marshal("")
}

func (o *Objective) UnmarshalJSON(data []byte) error {
	// TODO: create unmarshal method
	return nil
}

// ConstructObjectiveFromPayload takes in a message and constructs an objective from it.
// It accepts the message, myAddress, and a function to to retrieve ledgers from a store.
func ConstructObjectiveFromPayload(
	op protocols.ObjectivePayload,
	preApprove bool,
	getChannelFunc GetChannelByIdFunction,
) (Objective, error) {
	obj := Objective{}

	sp, err := getSwapPrimitivePayload(op.PayloadData)
	if err != nil {
		return Objective{}, fmt.Errorf("could not get swap primitive payload: %w", err)
	}

	if preApprove {
		obj.Status = protocols.Approved
	} else {
		obj.Status = protocols.Unapproved
	}

	obj.SwapPrimitive = sp

	ch, ok := getChannelFunc(sp.ChannelId)
	if !ok {
		return Objective{}, fmt.Errorf("unable to construct objective from payload, swap channel not found")
	}

	obj.C = &channel.SwapChannel{
		Channel: *ch,
	}

	return obj, nil
}

func getSwapPrimitivePayload(b []byte) (SwapPrimitive, error) {
	sp := SwapPrimitive{}
	err := json.Unmarshal(b, &sp)
	if err != nil {
		return sp, fmt.Errorf("could not unmarshal swap primitive: %w", err)
	}
	return sp, nil
}

// IsSwapObjective inspects a objective id and returns true if the objective id is for a swap objective.
func IsSwapObjective(id protocols.ObjectiveId) bool {
	return strings.HasPrefix(string(id), ObjectivePrefix)
}

// ObjectiveRequest represents a request to create a new virtual funding objective.
type ObjectiveRequest struct {
	ChannelId        types.Destination
	FromAsset        common.Address
	ToAsset          common.Address
	FromAmount       *big.Int
	ToAmount         *big.Int
	objectiveStarted chan struct{}
}

// NewObjectiveRequest creates a new ObjectiveRequest.
func NewObjectiveRequest(channelId types.Destination, fromAsset common.Address, toAsset common.Address, fromAmount *big.Int, toAmount *big.Int) ObjectiveRequest {
	return ObjectiveRequest{
		ChannelId:        channelId,
		FromAsset:        fromAsset,
		ToAsset:          toAsset,
		FromAmount:       fromAmount,
		ToAmount:         toAmount,
		objectiveStarted: make(chan struct{}),
	}
}

// Id returns the objective id for the request.
func (r ObjectiveRequest) Id(myAddress types.Address, chainId *big.Int) protocols.ObjectiveId {
	// TODO: Determine unique objective id
	return protocols.ObjectiveId(ObjectivePrefix + r.ChannelId.String())
}

// SignalObjectiveStarted is used by the engine to signal the objective has been started.
func (r ObjectiveRequest) SignalObjectiveStarted() {
	close(r.objectiveStarted)
}

// WaitForObjectiveToStart blocks until the objective starts
func (r ObjectiveRequest) WaitForObjectiveToStart() {
	<-r.objectiveStarted
}

// ObjectiveResponse is the type returned across the API in response to the ObjectiveRequest.
type ObjectiveResponse struct {
	Id        protocols.ObjectiveId
	ChannelId types.Destination
}

// Response computes and returns the appropriate response from the request.
func (r ObjectiveRequest) Response() ObjectiveResponse {
	return ObjectiveResponse{
		Id:        protocols.ObjectiveId(ObjectivePrefix + r.ChannelId.String()),
		ChannelId: r.ChannelId,
	}
}
