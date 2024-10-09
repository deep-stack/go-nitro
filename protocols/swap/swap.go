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
func NewObjective(request ObjectiveRequest, preApprove bool, isSwapSender bool, getChannelFunc GetChannelByIdFunction, address common.Address) (Objective, error) {
	// TODO: Handle objective creation for intermediary

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

	index, err := obj.determineSwapSenderIndex(address, isSwapSender)
	if err != nil {
		return obj, err
	}
	obj.SwapSenderIndex = index

	isValid := obj.isValidSwap(request.swap)
	if !isValid {
		return obj, fmt.Errorf("swap objective creation failed: %w", ErrInvalidSwap)
	}
	obj.Swap = request.swap
	obj.StateSigs = make(map[uint]state.Signature, 2)

	return obj, nil
}

func (o *Objective) determineSwapSenderIndex(myAddress common.Address, isSwapSender bool) (uint, error) {
	// state := o.C.LatestSupportedSwapChannelState()

	state, err := o.C.LatestSupportedState()
	if err != nil {
		return 0, err
	}

	var swapSenderIndex uint
	for allocationIndex, allocation := range state.Outcome[0].Allocations {
		if allocation.Destination == types.AddressToDestination(myAddress) {
			if isSwapSender {
				swapSenderIndex = uint(allocationIndex)
			} else {
				swapSenderIndex = uint(1 - allocationIndex)
			}
		}
	}
	return swapSenderIndex, nil
}

func (o *Objective) isValidSwap(swap payments.Swap) bool {
	tokenIn := swap.Exchange.TokenIn
	tokenOut := swap.Exchange.TokenOut

	if swap.Exchange.AmountIn.Cmp(big.NewInt(0)) < 0 {
		return false
	}

	if swap.Exchange.AmountOut.Cmp(big.NewInt(0)) < 0 {
		return false
	}

	// s := o.C.LatestSupportedSwapChannelState()

	s, err := o.C.LatestSupportedState()
	if err != nil {
		return false
	}
	updateSupportedState := s.Clone()
	updateOutcome := updateSupportedState.Outcome.Clone()

	for _, assetOutcome := range updateOutcome {
		swapSenderAllocation := assetOutcome.Allocations[o.SwapSenderIndex]
		swapReceiverAllocation := assetOutcome.Allocations[1-o.SwapSenderIndex]

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
	// TODO: Handle reject method for intermediary

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
	// TODO: Handle update method for intermediary

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

	myIndex := o.myIndexInAllocations()
	if myIndex == -1 {
		return &updated, fmt.Errorf("error in finding index")
	}
	counterPartySig := swapPayload.Swap.Sigs[uint(1-myIndex)]
	counterPartyAddress, err := o.Swap.RecoverSigner(counterPartySig)
	if err != nil {
		return &updated, err
	}

	if counterPartyAddress != o.counterPartyAddress() {
		return &updated, fmt.Errorf("swap lacks counterparty's signature")
	}

	updated.Swap = swapPayload.Swap

	// Ensure the incoming state sig is valid
	// counterPartyStateSig := swapPayload.StateSigs[1-updated.C.MyIndex]
	// state, err := updated.GetUpdatedSwapState()
	// if err != nil {
	// 	return &updated, err
	// }

	// counterPartyAddressFromStateSig, err := state.RecoverSigner(counterPartyStateSig)
	// if err != nil {
	// 	return &updated, err
	// }

	// if counterPartyAddressFromStateSig != o.otherParticipant() {
	// 	return &updated, fmt.Errorf("missing counterparty's signature in state signatures")
	// }

	updated.StateSigs = swapPayload.StateSigs
	updated.SwapStatus = swapPayload.SwapStatus

	return &updated, nil
}

// Crank inspects the extended state and declares a list of Effects to be executed
// It's like a state machine transition function where the finite / enumerable state is returned (computed from the extended state)
// rather than being independent of the extended state; and where there is only one type of event ("the crank") with no data on it at all.
func (o *Objective) Crank(secretKey *[]byte) (protocols.Objective, protocols.SideEffects, protocols.WaitingFor, error) {
	// TODO: Handle crank method for intermediary

	updated := o.clone()

	sideEffects := protocols.SideEffects{}
	// Input validation
	if updated.Status != protocols.Approved {
		return &updated, sideEffects, WaitingForNothing, protocols.ErrNotApproved
	}
	// TODO: Both participant checks whether swap operation is valid

	// TODO: Swap receiver check whether to accept or reject
	if updated.SwapSenderIndex != o.C.MyIndex && updated.SwapStatus != types.Accepted {
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

		index := o.myIndexInAllocations()
		if index == -1 {
			return &updated, protocols.SideEffects{}, WaitingForConsensus, fmt.Errorf("error in finding index")
		}
		err = updated.Swap.AddSignature(sig, uint(index))
		if err != nil {
			return &updated, sideEffects, WaitingForConsensus, err
		}

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
	err := updated.UpdateSwapChannelState()
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
	// fmt.Println("UPDATED STATE AFTER SWAP OUTCOME", updatedState.Outcome)

	updatedSignedState := state.NewSignedState(updatedState)
	for _, sig := range o.StateSigs {
		err := updatedSignedState.AddSignature(sig)
		if err != nil {
			return fmt.Errorf("error adding signature to signed swap channel state %w", err)
		}
	}

	fmt.Println("\nUPDATED SIGNED STATE AFTER SWAP OUTCOME", updatedSignedState.State().Outcome)
	ok := o.C.AddSignedSwapChannelState(updatedSignedState)
	if !ok {
		return fmt.Errorf("error adding signed state to swap channel %w", err)
	}
	state := o.C.LatestSupportedSwapChannelState()
	fmt.Println("\nLATEST STATE OUTCOME AFTER ADDING STATE TO CHANNEL", state.Outcome)

	return nil
}

func (o *Objective) GetUpdatedSwapState() (state.State, error) {
	tokenIn := o.Swap.Exchange.TokenIn
	tokenOut := o.Swap.Exchange.TokenOut

	// s := o.C.LatestSupportedSwapChannelState()
	s, err := o.C.LatestSupportedState()
	if err != nil {
		return state.State{}, fmt.Errorf("latest supported state not found: %w", err)
	}
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
	myIndex := o.myIndexInAllocations()
	if myIndex == -1 {
		return false
	}
	_, found := o.Swap.Sigs[uint(myIndex)]
	return found
}

// TODO: Remove
func (o *Objective) FindParticipantIndex(address common.Address) (int, error) {
	for index, participantAddress := range o.C.Participants {
		if participantAddress == address {
			return index, nil
		}
	}

	return -1, fmt.Errorf("participant not found")
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

	obj, err := NewObjective(objectiveReq, preApprove, false, getChannelFunc, address)
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

func (o *Objective) counterPartyAddress() common.Address {
	length := len(o.C.Participants)

	if o.C.MyIndex == 0 {
		return o.C.Participants[length-1]
	} else if o.C.MyIndex == uint(length-1) {
		return o.C.Participants[0]
	}

	return common.Address{}
}

func (o *Objective) myIndexInAllocations() int {
	myIndex := -1
	myAddress := o.C.Participants[o.C.MyIndex]
	// state := o.C.LatestSupportedSwapChannelState()
	state, _ := o.C.LatestSupportedState()
	for i, allocation := range state.Outcome[0].Allocations {
		if allocation.Destination == types.AddressToDestination(myAddress) {
			myIndex = i
		}
	}
	return myIndex
}
