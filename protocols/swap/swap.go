package swap

import (
	"encoding/json"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/statechannels/go-nitro/channel"
	"github.com/statechannels/go-nitro/channel/state"
	"github.com/statechannels/go-nitro/protocols"
	"github.com/statechannels/go-nitro/types"
)

type Exchange struct {
	FromAsset  common.Address
	ToAsset    common.Address
	FromAmount *big.Int
	ToAmount   *big.Int
}

type SwapPrimitive struct {
	channelId types.Destination
	exchange  Exchange
	sigs      map[uint]state.Signature // keyed by participant index in swap channel
}

func NewSwap(channelId types.Destination, fromAsset, toAsset common.Address, fromAmount, toAmout *big.Int) SwapPrimitive {
	return SwapPrimitive{
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
	Status protocols.ObjectiveStatus
	C      *channel.SwapChannel

	MyRole uint // index in the swap protocol
}

// NewObjective creates a new swap objective from a given request.
func NewObjective(request ObjectiveRequest, preApprove bool, myAddress types.Address) (Objective, error) {
	return Objective{}, nil
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

	messages := protocols.CreateRejectionNoticeMessage(o.Id(), o.otherParticipants()...)
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

func (o *Objective) otherParticipants() []types.Address {
	otherParticipants := make([]types.Address, 0)
	for i, p := range o.C.Participants {
		if i != int(o.MyRole) {
			otherParticipants = append(otherParticipants, p)
		}
	}
	return otherParticipants
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
	p protocols.ObjectivePayload,
	preapprove bool,
	myAddress types.Address,
) (Objective, error) {
	return Objective{}, nil
}

// IsSwapObjective inspects a objective id and returns true if the objective id is for a swap objective.
func IsSwapObjective(id protocols.ObjectiveId) bool {
	return strings.HasPrefix(string(id), ObjectivePrefix)
}

// ObjectiveRequest represents a request to create a new virtual funding objective.
type ObjectiveRequest struct {
	objectiveStarted chan struct{}
}

// NewObjectiveRequest creates a new ObjectiveRequest.
func NewObjectiveRequest() ObjectiveRequest {
	return ObjectiveRequest{
		objectiveStarted: make(chan struct{}),
	}
}

// Id returns the objective id for the request.
func (r ObjectiveRequest) Id(myAddress types.Address, chainId *big.Int) protocols.ObjectiveId {
	// TODO: Determine objective id
	return protocols.ObjectiveId(ObjectivePrefix + "")
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
		Id:        protocols.ObjectiveId(ObjectivePrefix + ""),
		ChannelId: types.Destination{},
	}
}
