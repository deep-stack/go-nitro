package bridgeddefund

import (
	"math/big"

	"github.com/statechannels/go-nitro/channel"
	"github.com/statechannels/go-nitro/protocols"
	"github.com/statechannels/go-nitro/types"
)

const ObjectivePrefix = "bridgeddefunding-"

// Objective is a cache of data computed by reading from the store. It stores (potentially) infinite data
type Objective struct {
	Status protocols.ObjectiveStatus
	C      *channel.Channel
}

// TODO: Implement new objective method
func NewObjective(
	request ObjectiveRequest,
) (Objective, error) {
	ch := channel.Channel{Id: request.ChannelId}
	return Objective{
		C: &ch,
	}, nil
}

// GetStatus returns the status of the objective.
func (o *Objective) GetStatus() protocols.ObjectiveStatus {
	return o.Status
}

// OwnsChannel returns the channel that the objective is funding.
func (o *Objective) OwnsChannel() types.Destination {
	return o.C.Id
}

func (o *Objective) Related() []protocols.Storable {
	return []protocols.Storable{o.C}
}

func (o *Objective) Id() protocols.ObjectiveId {
	return protocols.ObjectiveId(ObjectivePrefix + o.C.Id.String())
}

// Crank inspects the extended state and declares a list of Effects to be executed
// It's like a state machine transition function where the finite / enumerable state is returned (computed from the extended state)
// rather than being independent of the extended state; and where there is only one type of event ("the crank") with no data on it at all
func (o *Objective) Crank(secretKey *[]byte) (protocols.Objective, protocols.SideEffects, protocols.WaitingFor, error) {
	// TODO: Implement crank method
	return o, protocols.SideEffects{}, "", nil
}

func (o *Objective) Approve() protocols.Objective {
	// TODO: Implement approve method
	return o
}

func (o *Objective) Reject() (protocols.Objective, protocols.SideEffects) {
	// TODO: Implement reject method
	return o, protocols.SideEffects{}
}

// Update receives an ObjectivePayload, applies all applicable data to the BridgedDeFundingObjectiveState,
// and returns the updated state
func (o *Objective) Update(p protocols.ObjectivePayload) (protocols.Objective, error) {
	// TODO: Implement update method
	return o, nil
}

// ObjectiveRequest represents a request to create a new bridged defund objective.
type ObjectiveRequest struct {
	ChannelId        types.Destination
	objectiveStarted chan struct{}
}

// NewObjectiveRequest creates a new ObjectiveRequest.
func NewObjectiveRequest(channelId types.Destination) ObjectiveRequest {
	return ObjectiveRequest{
		ChannelId:        channelId,
		objectiveStarted: make(chan struct{}),
	}
}

func (r ObjectiveRequest) SignalObjectiveStarted() {
	close(r.objectiveStarted)
}

// WaitForObjectiveToStart blocks until the objective starts
func (r ObjectiveRequest) WaitForObjectiveToStart() {
	<-r.objectiveStarted
}

// Id returns the objective id for the request.
func (r ObjectiveRequest) Id(myAddress types.Address, chainId *big.Int) protocols.ObjectiveId {
	return protocols.ObjectiveId(ObjectivePrefix + r.ChannelId.String())
}
