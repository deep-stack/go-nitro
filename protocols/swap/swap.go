package swap

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/statechannels/go-nitro/channel"
	"github.com/statechannels/go-nitro/channel/state"
	"github.com/statechannels/go-nitro/payments"
	"github.com/statechannels/go-nitro/protocols"
	"github.com/statechannels/go-nitro/types"
)

type GetChannelByIdFunction func(id types.Destination) (channel *channel.Channel, ok bool)

const (
	SwapPayloadType protocols.PayloadType = "SwapPayload"
)

const (
	WaitingForConsensus    protocols.WaitingFor = "WaitingForConsensus"
	WaitingForConfirmation protocols.WaitingFor = "WaitingForConfirmation"
	WaitingForNothing      protocols.WaitingFor = "WaitingForNothing" // Finished
)

const ObjectivePrefix = "Swap-"

var (
	ErrInvalidSwap error = errors.New("invalid swap")
	ErrSwapExists  error = errors.New("swap already exists")
)

type SwapPayload struct {
	Swap       payments.Swap
	StateSigs  map[uint]state.Signature
	SwapStatus types.SwapStatus
}

// Objective is a cache of data computed by reading from the store. It stores (potentially) infinite data.
type Objective struct {
	Status     protocols.ObjectiveStatus
	C          *channel.SwapChannel
	Swap       payments.Swap
	StateSigs  map[uint]state.Signature
	SwapStatus types.SwapStatus
	// Index of participant who initiated the swap in allocations array
	SwapSenderIndex uint
}

// NewObjective creates a new swap objective from a given request.
func NewObjective(request ObjectiveRequest, preApprove bool, isSwapSender bool, getChannelFunc GetChannelByIdFunction) (Objective, error) {
	obj := Objective{}

	swapChannel, ok := getChannelFunc(request.swap.ChannelId)
	if !ok {
		return obj, fmt.Errorf("swap objective creation failed, swap channel not found")
	}

	obj.C = &channel.SwapChannel{
		Channel: *swapChannel,
	}

	if preApprove {
		obj.Status = protocols.Approved
	} else {
		obj.Status = protocols.Unapproved
	}

	index, err := obj.determineSwapSenderIndex(isSwapSender)
	if err != nil {
		return obj, err
	}
	obj.SwapSenderIndex = index

	s := obj.C.LatestSupportedSwapChannelState()

	isValid := IsValidSwap(s, request.swap, obj.SwapSenderIndex)
	if !isValid {
		return obj, fmt.Errorf("swap objective creation failed: %w", ErrInvalidSwap)
	}
	obj.Swap = request.swap
	obj.StateSigs = make(map[uint]state.Signature, 2)

	return obj, nil
}

func (o *Objective) determineSwapSenderIndex(isSwapSender bool) (uint, error) {
	myIndex, err := MyIndexInAllocations(o.C)
	if err != nil {
		return 0, err
	}

	var swapSenderIndex uint
	if isSwapSender {
		swapSenderIndex = myIndex
	} else {
		swapSenderIndex = 1 - myIndex
	}

	return swapSenderIndex, nil
}

func IsValidSwap(s state.State, swap payments.Swap, swapSenderIndex uint) bool {
	tokenIn := swap.Exchange.TokenIn
	tokenOut := swap.Exchange.TokenOut

	if swap.Exchange.AmountIn.Cmp(big.NewInt(0)) < 0 {
		return false
	}

	if swap.Exchange.AmountOut.Cmp(big.NewInt(0)) < 0 {
		return false
	}

	updateSupportedState := s.Clone()
	updateOutcome := updateSupportedState.Outcome.Clone()

	for _, assetOutcome := range updateOutcome {
		swapSenderAllocation := assetOutcome.Allocations[swapSenderIndex]
		swapReceiverAllocation := assetOutcome.Allocations[1-swapSenderIndex]

		if assetOutcome.Asset == tokenIn {
			res := swapSenderAllocation.Amount.Sub(swapSenderAllocation.Amount, swap.Exchange.AmountIn)
			if res.Cmp(big.NewInt(0)) < 0 {
				return false
			}
		}

		if assetOutcome.Asset == tokenOut {
			res := swapReceiverAllocation.Amount.Sub(swapReceiverAllocation.Amount, swap.Exchange.AmountOut)
			if res.Cmp(big.NewInt(0)) < 0 {
				return false
			}
		}
	}

	return true
}

// Id returns the objective id.
func (o *Objective) Id() protocols.ObjectiveId {
	return protocols.ObjectiveId(ObjectivePrefix + o.Swap.Id.String())
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
	updated.SwapStatus = types.Rejected

	peer := o.counterPartyAddress()
	messages := protocols.CreateRejectionNoticeMessage(o.Id(), peer)
	sideEffects := protocols.SideEffects{MessagesToSend: messages}
	return &updated, sideEffects
}

// OwnsChannel returns the channel that the objective is funding.
func (o *Objective) OwnsChannel() types.Destination {
	// Swap objective doesnt owns any channel
	return types.Destination{}
}

// GetStatus returns the status of the objective.
func (o *Objective) GetStatus() protocols.ObjectiveStatus {
	return o.Status
}

// Update receives an protocols.ObjectiveEvent, applies all applicable event data to the VirtualFundObjective,
// and returns the updated state.
func (o *Objective) Update(raw protocols.ObjectivePayload) (protocols.Objective, error) {
	updated := o.clone()
	swapPayload, err := getSwapPayload(raw.PayloadData)
	if err != nil {
		return &updated, fmt.Errorf("could not get swap payload: %w", err)
	}

	// Ensure the incoming swap is valid
	ok := o.Swap.Equal(swapPayload.Swap)
	if !ok {
		return &updated, fmt.Errorf("swap does not match")
	}

	counterPartySig := swapPayload.Swap.Sigs[uint(o.counterPartyIndexInParticipants())]
	counterPartyAddress, err := o.Swap.RecoverSigner(counterPartySig)
	if err != nil {
		return &updated, err
	}

	if counterPartyAddress != o.counterPartyAddress() {
		return &updated, fmt.Errorf("swap lacks counterparty's signature")
	}

	updated.Swap = swapPayload.Swap

	// Ensure the incoming state sig is valid
	counterPartyStateSig := swapPayload.StateSigs[uint(o.counterPartyIndexInParticipants())]
	state, err := updated.GetUpdatedSwapState()
	if err != nil {
		return &updated, err
	}

	counterPartyAddressFromStateSig, err := state.RecoverSigner(counterPartyStateSig)
	if err != nil {
		return &updated, fmt.Errorf("error in recovering counter party address %w", err)
	}

	if counterPartyAddressFromStateSig != o.counterPartyAddress() {
		return &updated, fmt.Errorf("missing counterparty's signature in state signatures")
	}

	updated.StateSigs = swapPayload.StateSigs
	updated.SwapStatus = swapPayload.SwapStatus

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

	myIndex, err := MyIndexInAllocations(o.C)
	if err != nil {
		return &updated, sideEffects, WaitingForNothing, err
	}

	// Swap receiver checks whether to accept or reject
	if updated.SwapSenderIndex != myIndex && updated.SwapStatus != types.Accepted {
		if updated.SwapStatus == types.PendingConfirmation {
			return &updated, sideEffects, WaitingForConfirmation, nil
		} else {
			// Rejected
			updated.Status = protocols.Completed
			o, sideEffects := updated.Reject()
			return o, sideEffects, WaitingForNothing, nil
		}
	}

	// Verify if I have signed it
	// If not, sign it and send it to the counterparty
	if !updated.HasSignatureForParticipant() {
		sig, err := updated.Swap.Sign(*secretKey)
		if err != nil {
			return &updated, sideEffects, WaitingForConsensus, err
		}

		updated.Swap.AddSignature(sig, updated.C.MyIndex)
		updatedState, err := updated.GetUpdatedSwapState()
		if err != nil {
			return &updated, protocols.SideEffects{}, WaitingForConsensus, fmt.Errorf("error creating updated swap channel state %w", err)
		}

		stateSig, err := updatedState.Sign(*secretKey)
		if err != nil {
			return &updated, sideEffects, WaitingForConsensus, fmt.Errorf("error signing swap channel state %w", err)
		}

		updated.StateSigs[updated.C.MyIndex] = stateSig

		messages, err := protocols.CreateObjectivePayloadMessage(
			updated.Id(),
			SwapPayload{
				Swap:       updated.Swap,
				StateSigs:  updated.StateSigs,
				SwapStatus: updated.SwapStatus,
			},
			SwapPayloadType,
			o.counterPartyAddress(),
		)
		if err != nil {
			return &updated, protocols.SideEffects{}, WaitingForConsensus, fmt.Errorf("could not create payload message %w", err)
		}

		sideEffects.MessagesToSend = append(sideEffects.MessagesToSend, messages...)
	}

	// Wait if all signatures are not available
	if !updated.HasAllSignatures() {
		return &updated, sideEffects, WaitingForConsensus, nil
	}

	// If all signatures are available, update the swap channel according to the swap
	err = updated.UpdateSwapChannelState()
	if err != nil {
		return &updated, protocols.SideEffects{}, WaitingForConsensus, fmt.Errorf("error updating swap channel state %w", err)
	}

	// Completion
	updated.Status = protocols.Completed
	return &updated, sideEffects, WaitingForNothing, nil
}

func (o *Objective) UpdateSwapChannelState() error {
	updatedState, err := o.GetUpdatedSwapState()
	if err != nil {
		return fmt.Errorf("error creating updated swap channel state %w", err)
	}

	updatedSignedState := state.NewSignedState(updatedState)
	for _, sig := range o.StateSigs {
		err := updatedSignedState.AddSignature(sig)
		if err != nil {
			return fmt.Errorf("error adding signature to signed swap channel state %w", err)
		}
	}

	ok := o.C.AddSignedSwapChannelState(updatedSignedState)
	if !ok {
		return fmt.Errorf("error adding signed state to swap channel %w", err)
	}

	return nil
}

func (o *Objective) GetUpdatedSwapState() (state.State, error) {
	tokenIn := o.Swap.Exchange.TokenIn
	tokenOut := o.Swap.Exchange.TokenOut

	s := o.C.LatestSupportedSwapChannelState()
	updateSupportedState := s.Clone()
	updateOutcome := updateSupportedState.Outcome.Clone()

	for _, assetOutcome := range updateOutcome {

		swapSenderAllocation := assetOutcome.Allocations[o.SwapSenderIndex]
		swapReceiverAllocation := assetOutcome.Allocations[1-o.SwapSenderIndex]

		if assetOutcome.Asset == tokenIn {

			swapSenderAllocation.Amount.Sub(swapSenderAllocation.Amount, o.Swap.Exchange.AmountIn)
			swapReceiverAllocation.Amount.Add(swapReceiverAllocation.Amount, o.Swap.Exchange.AmountIn)
		}

		if assetOutcome.Asset == tokenOut {
			swapSenderAllocation.Amount.Add(swapSenderAllocation.Amount, o.Swap.Exchange.AmountOut)
			swapReceiverAllocation.Amount.Sub(swapReceiverAllocation.Amount, o.Swap.Exchange.AmountOut)
		}
	}

	updateSupportedState.Outcome = updateOutcome
	updateSupportedState.TurnNum++
	return updateSupportedState, nil
}

func (o *Objective) Related() []protocols.Storable {
	ret := []protocols.Storable{o.C, &o.Swap}

	return ret
}

// Clone returns a deep copy of the receiver.
func (o *Objective) clone() Objective {
	clone := Objective{}
	clone.Status = o.Status
	clone.SwapSenderIndex = o.SwapSenderIndex
	clone.SwapStatus = o.SwapStatus
	clone.C = o.C.Clone()
	clone.Swap = o.Swap.Clone()

	clonedSigs := make(map[uint]state.Signature, len(o.StateSigs))
	for i, sig := range o.StateSigs {
		clonedSigs[i] = sig
	}
	clone.StateSigs = clonedSigs

	return clone
}

// HasAllSignatures returns true if every participant has a valid signature.
func (o *Objective) HasAllSignatures() bool {
	// Since signatures are validated
	if len(o.Swap.Sigs) == 2 {
		return true
	} else {
		return false
	}
}

// HasSignatureForParticipant returns true if the participant (at participantIndex) has a valid signature.
func (o *Objective) HasSignatureForParticipant() bool {
	_, found := o.Swap.Sigs[o.C.MyIndex]
	return found
}

func (o *Objective) AcceptSwap() {
	o.SwapStatus = types.Accepted
}

type jsonObjective struct {
	Status          protocols.ObjectiveStatus
	C               types.Destination
	Swap            types.Destination
	SwapSenderIndex uint
	SwapStatus      types.SwapStatus
	Nonce           uint64
	StateSigs       map[uint]state.Signature
}

func (o Objective) MarshalJSON() ([]byte, error) {
	jsonSO := jsonObjective{
		Status:          o.Status,
		C:               o.C.Id,
		Swap:            o.Swap.Id,
		SwapStatus:      o.SwapStatus,
		SwapSenderIndex: o.SwapSenderIndex,
		StateSigs:       o.StateSigs,
	}
	return json.Marshal(jsonSO)
}

func (o *Objective) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}

	var jsonSo jsonObjective
	if err := json.Unmarshal(data, &jsonSo); err != nil {
		return fmt.Errorf("failed to unmarshal the Swap Objective: %w", err)
	}

	o.Status = jsonSo.Status
	o.SwapSenderIndex = jsonSo.SwapSenderIndex
	o.SwapStatus = jsonSo.SwapStatus
	o.Swap = payments.Swap{}
	o.Swap.Id = jsonSo.Swap
	o.StateSigs = jsonSo.StateSigs

	o.C = &channel.SwapChannel{}
	o.C.Id = jsonSo.C

	return nil
}

// ConstructObjectiveFromPayload takes in a message and constructs an objective from it.
// It accepts the message, myAddress, and a function to to retrieve ledgers from a store.
func ConstructObjectiveFromPayload(
	op protocols.ObjectivePayload,
	preApprove bool,
	getChannelFunc GetChannelByIdFunction,
	address common.Address,
) (Objective, error) {
	swapPayload, err := getSwapPayload(op.PayloadData)
	if err != nil {
		return Objective{}, fmt.Errorf("could not get swap payload: %w", err)
	}

	objectiveReq := ObjectiveRequest{
		swap: swapPayload.Swap,
	}

	obj, err := NewObjective(objectiveReq, preApprove, false, getChannelFunc)
	if err != nil {
		return Objective{}, fmt.Errorf("unable to construct swap objective from payload: %w", err)
	}

	return obj, nil
}

func getSwapPayload(b []byte) (SwapPayload, error) {
	payload := SwapPayload{}
	err := json.Unmarshal(b, &payload)
	if err != nil {
		return payload, fmt.Errorf("could not unmarshal swap: %w", err)
	}
	return payload, nil
}

// IsSwapObjective inspects a objective id and returns true if the objective id is for a swap objective.
func IsSwapObjective(id protocols.ObjectiveId) bool {
	return strings.HasPrefix(string(id), ObjectivePrefix)
}

// ObjectiveRequest represents a request to create a new swap objective.
type ObjectiveRequest struct {
	swap             payments.Swap
	objectiveStarted chan struct{}
}

// NewObjectiveRequest creates a new ObjectiveRequest.
func NewObjectiveRequest(channelId types.Destination, tokenIn common.Address, tokenOut common.Address, amountIn *big.Int, amountOut *big.Int, fixedPart state.FixedPart, nonce uint64) ObjectiveRequest {
	swap := payments.NewSwap(channelId, tokenIn, tokenOut, amountIn, amountOut, nonce)
	return ObjectiveRequest{
		swap:             swap,
		objectiveStarted: make(chan struct{}),
	}
}

// Id returns the objective id for the request.
func (r ObjectiveRequest) Id(myAddress types.Address, chainId *big.Int) protocols.ObjectiveId {
	return protocols.ObjectiveId(ObjectivePrefix + r.swap.SwapId().String())
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
		Id:        protocols.ObjectiveId(ObjectivePrefix + r.swap.Id.String()),
		ChannelId: r.swap.ChannelId,
	}
}

func (o *Objective) counterPartyIndexInParticipants() int {
	length := len(o.C.Participants)

	if o.C.MyIndex == 0 {
		return length - 1
	} else if o.C.MyIndex == uint(length-1) {
		return 0
	}

	return -1
}

func (o *Objective) counterPartyAddress() common.Address {
	counterPartyIndex := o.counterPartyIndexInParticipants()

	if counterPartyIndex == -1 {
		return common.Address{}
	}

	return o.C.Participants[counterPartyIndex]
}

func MyIndexInAllocations(c *channel.SwapChannel) (uint, error) {
	myAddress := c.Participants[c.MyIndex]
	state, _ := c.LatestSupportedState()
	for i, allocation := range state.Outcome[0].Allocations {
		if allocation.Destination == types.AddressToDestination(myAddress) {
			return uint(i), nil
		}
	}
	return 0, fmt.Errorf("unable to find participant's address (%s) in the allocations", myAddress)
}
