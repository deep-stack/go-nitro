package swap

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	ethAbi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/statechannels/go-nitro/abi"
	"github.com/statechannels/go-nitro/channel"
	"github.com/statechannels/go-nitro/channel/state"
	nc "github.com/statechannels/go-nitro/crypto"
	"github.com/statechannels/go-nitro/protocols"
	"github.com/statechannels/go-nitro/types"
)

type GetChannelByIdFunction func(id types.Destination) (channel *channel.Channel, ok bool)

const (
	SwapPayloadType protocols.PayloadType = "SwapPayload"
)

const (
	WaitingForSwapping protocols.WaitingFor = "WaitingForSwapping"
	WaitingForNothing  protocols.WaitingFor = "WaitingForNothing" // Finished
)

const ObjectivePrefix = "Swap-"

type Exchange struct {
	TokenIn   common.Address
	TokenOut  common.Address
	AmountIn  *big.Int
	AmountOut *big.Int
}

func (ex Exchange) Equal(target Exchange) bool {
	return ex.TokenIn == target.TokenIn && ex.TokenOut == target.TokenOut && ex.AmountIn.Cmp(target.AmountIn) == 0 && ex.AmountOut.Cmp(target.AmountOut) == 0
}

type SwapPayload struct {
	SwapPrimitive SwapPrimitive
	StateSigs     map[uint]state.Signature
}

type SwapPrimitive struct {
	ChannelId types.Destination
	Exchange  Exchange
	Sigs      map[uint]state.Signature // keyed by participant index in swap channel
	Nonce     uint64
}

func NewSwapPrimitive(channelId types.Destination, tokenIn, tokenOut common.Address, amountIn, amountOut *big.Int, nonce uint64) SwapPrimitive {
	return SwapPrimitive{
		ChannelId: channelId,
		Exchange: Exchange{
			tokenIn,
			tokenOut,
			amountIn,
			amountOut,
		},
		Sigs:  make(map[uint]state.Signature, 2),
		Nonce: nonce,
	}
}

// TODO: Check need of custom marshall and unmarshall methods for swap primitive

// encodes the state into a []bytes value
func (sp SwapPrimitive) encode() (types.Bytes, error) {
	// TODO: Check whether we need to encode array of swap primitive
	// TODO: Check need of app data for sad path will be array of swap primitive
	return ethAbi.Arguments{
		{Type: abi.Destination}, // channel id
		{Type: abi.Address},     // tokenIn
		{Type: abi.Address},     // tokenOut
		{Type: abi.Uint256},     // amountIn
		{Type: abi.Uint256},     // amountOut
		{Type: abi.Uint256},     // nonce
	}.Pack(
		sp.ChannelId,
		sp.Exchange.TokenIn,
		sp.Exchange.TokenOut,
		sp.Exchange.AmountIn,
		sp.Exchange.AmountOut,
		new(big.Int).SetUint64(sp.Nonce),
	)
}

func (sp SwapPrimitive) Equal(target SwapPrimitive) bool {
	return sp.ChannelId == target.ChannelId && sp.Exchange.Equal(target.Exchange) && sp.Nonce == target.Nonce
}

func (sp SwapPrimitive) Clone() SwapPrimitive {
	clonedSigs := make(map[uint]state.Signature, len(sp.Sigs))
	for i, sig := range sp.Sigs {
		clonedSigs[i] = sig
	}

	return SwapPrimitive{
		ChannelId: sp.ChannelId,
		Exchange: Exchange{
			TokenIn:   sp.Exchange.TokenIn,
			TokenOut:  sp.Exchange.TokenOut,
			AmountIn:  sp.Exchange.AmountIn,
			AmountOut: sp.Exchange.AmountOut,
		},
		Sigs:  clonedSigs,
		Nonce: sp.Nonce,
	}
}

// Hash returns the keccak256 hash of the State
func (sp SwapPrimitive) Hash() (types.Bytes32, error) {
	encoded, err := sp.encode()
	if err != nil {
		return types.Bytes32{}, fmt.Errorf("failed to encode swap primitive: %w", err)
	}
	return crypto.Keccak256Hash(encoded), nil
}

// Sign generates an ECDSA signature on the swap primitive using the supplied private key
func (sp SwapPrimitive) Sign(secretKey []byte) (state.Signature, error) {
	hash, error := sp.Hash()
	if error != nil {
		return state.Signature{}, error
	}
	return nc.SignEthereumMessage(hash.Bytes(), secretKey)
}

func (sp SwapPrimitive) AddSignature(sig state.Signature, myIndex uint) error {
	sp.Sigs[myIndex] = sig
	return nil
}

// RecoverSigner computes the Ethereum address which generated Signature sig on State state
func (sp SwapPrimitive) RecoverSigner(sig state.Signature) (types.Address, error) {
	hash, error := sp.Hash()
	if error != nil {
		return types.Address{}, error
	}
	return nc.RecoverEthereumMessageSigner(hash[:], sig)
}

// Objective is a cache of data computed by reading from the store. It stores (potentially) infinite data.
type Objective struct {
	Status        protocols.ObjectiveStatus
	C             *channel.SwapChannel
	SwapPrimitive SwapPrimitive
	StateSigs     map[uint]state.Signature
	SwapperIndex  uint
}

// NewObjective creates a new swap objective from a given request.
func NewObjective(request ObjectiveRequest, preApprove bool, isSwapper bool, getChannelFunc GetChannelByIdFunction, address common.Address) (Objective, error) {
	obj := Objective{}

	swapChannel, ok := getChannelFunc(request.channelId)
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

	myIndex, err := obj.FindParticipantIndex(address)
	if err != nil {
		return obj, err
	}

	if isSwapper {
		swapPrimitive := NewSwapPrimitive(request.channelId, request.tokenIn, request.tokenOut, request.amountIn, request.amountOut, request.nonce)
		obj.SwapPrimitive = swapPrimitive
		obj.SwapperIndex = uint(myIndex)
	} else {
		obj.SwapperIndex = 1 - uint(myIndex)
	}

	obj.StateSigs = make(map[uint]state.Signature, 2)

	return obj, nil
}

// Id returns the objective id.
func (o *Objective) Id() protocols.ObjectiveId {
	spId := getSwapPrimitiveId(o.C.FixedPart, o.SwapPrimitive.Nonce)
	return protocols.ObjectiveId(ObjectivePrefix + spId.String())
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
	// TODO: Check objective id are same

	updated := o.clone()

	sp, err := getSwapPayload(raw.PayloadData)
	if err != nil {
		return &updated, fmt.Errorf("could not get swap primitive payload: %w", err)
	}

	// Ensure the incoming swap primitive is valid
	ok := o.SwapPrimitive.Equal(sp.SwapPrimitive)
	if !ok {
		return &updated, fmt.Errorf("swap primitive does not match")
	}

	counterPartySig := sp.SwapPrimitive.Sigs[1-updated.C.MyIndex]
	counterPartyAddress, err := o.SwapPrimitive.RecoverSigner(counterPartySig)
	if err != nil {
		return &updated, err
	}

	if counterPartyAddress != o.C.Participants[1-o.C.MyIndex] {
		return &updated, fmt.Errorf("swap primitive lacks counterparty's signature")
	}

	updated.SwapPrimitive = sp.SwapPrimitive
	// TODO: Validation for state sigs
	updated.StateSigs = sp.StateSigs

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
	// TODO: Both participant checks whether swap operation is valid one

	// TODO: Swapee check whether to accept or reject

	// Verify if I have signed it
	// If not, sign it and send it to the counterparty
	if !updated.HasSignatureForParticipant() {
		sig, err := updated.SwapPrimitive.Sign(*secretKey)
		if err != nil {
			return &updated, sideEffects, WaitingForSwapping, err
		}

		err = updated.SwapPrimitive.AddSignature(sig, updated.C.MyIndex)
		if err != nil {
			return &updated, sideEffects, WaitingForSwapping, err
		}

		updatedState, err := updated.GetUpdatedSwapState()
		if err != nil {
			return &updated, protocols.SideEffects{}, WaitingForSwapping, fmt.Errorf("error creating updated swap channel state %w", err)
		}

		stateSig, err := updatedState.Sign(*secretKey)
		if err != nil {
			return &updated, sideEffects, WaitingForSwapping, fmt.Errorf("error signing swap channel state %w", err)
		}

		updated.StateSigs[updated.C.MyIndex] = stateSig

		messages, err := protocols.CreateObjectivePayloadMessage(
			updated.Id(),
			SwapPayload{
				SwapPrimitive: updated.SwapPrimitive,
				StateSigs:     updated.StateSigs,
			},
			SwapPayloadType,
			o.C.Participants[1-o.C.MyIndex],
		)
		if err != nil {
			return &updated, protocols.SideEffects{}, WaitingForSwapping, fmt.Errorf("could not create payload message %w", err)
		}

		sideEffects.MessagesToSend = append(sideEffects.MessagesToSend, messages...)
	}

	// Wait if all signatures are not available
	if !updated.HasAllSignatures() {
		return &updated, sideEffects, WaitingForSwapping, nil
	}

	// If all signatures are available, update the swap channel according to the swap primitive and add the swap primitive to the swap channel
	err := updated.UpdateSwapChannelState()
	if err != nil {
		return &updated, protocols.SideEffects{}, WaitingForSwapping, fmt.Errorf("error updating swap channel state %w", err)
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

	ok := o.C.AddSignedState(updatedSignedState)
	if !ok {
		return fmt.Errorf("error adding signed state to swap channel %w", err)
	}

	return nil
}

func (o *Objective) GetUpdatedSwapState() (state.State, error) {
	tokenIn := o.SwapPrimitive.Exchange.TokenIn
	tokenOut := o.SwapPrimitive.Exchange.TokenOut

	s, err := o.C.LatestSupportedState()
	if err != nil {
		return state.State{}, fmt.Errorf("latest supported state not found: %w", err)
	}
	updateSupportedState := s.Clone()
	updateOutcome := updateSupportedState.Outcome.Clone()

	for _, assetOutcome := range updateOutcome {

		swapperAllocation := assetOutcome.Allocations[o.SwapperIndex]
		swappeAllocation := assetOutcome.Allocations[1-o.SwapperIndex]

		if assetOutcome.Asset == tokenIn {

			swapperAllocation.Amount.Sub(swapperAllocation.Amount, o.SwapPrimitive.Exchange.AmountIn)
			swappeAllocation.Amount.Add(swappeAllocation.Amount, o.SwapPrimitive.Exchange.AmountIn)
		}

		if assetOutcome.Asset == tokenOut {
			swapperAllocation.Amount.Add(swapperAllocation.Amount, o.SwapPrimitive.Exchange.AmountOut)
			swappeAllocation.Amount.Sub(swappeAllocation.Amount, o.SwapPrimitive.Exchange.AmountOut)
		}
	}

	updateSupportedState.Outcome = updateOutcome
	updateSupportedState.TurnNum++
	return updateSupportedState, nil
}

func (o *Objective) Related() []protocols.Storable {
	ret := []protocols.Storable{o.C}

	return ret
}

// Clone returns a deep copy of the receiver.
func (o *Objective) clone() Objective {
	clone := Objective{}
	clone.Status = o.Status
	clone.SwapperIndex = o.SwapperIndex
	clone.C = o.C.Clone()
	clone.SwapPrimitive = o.SwapPrimitive.Clone()

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
	if len(o.SwapPrimitive.Sigs) == len(o.C.Participants) && len(o.StateSigs) == len(o.C.Participants) {
		return true
	} else {
		return false
	}
}

// HasSignatureForParticipant returns true if the participant (at participantIndex) has a valid signature.
func (o *Objective) HasSignatureForParticipant() bool {
	_, found := o.SwapPrimitive.Sigs[o.C.MyIndex]
	return found
}

func (o *Objective) FindParticipantIndex(address common.Address) (int, error) {
	for index, participantAddress := range o.C.Participants {
		if participantAddress == address {
			return index, nil
		}
	}

	return -1, fmt.Errorf("participant not found")
}

type jsonObjective struct {
	Status        protocols.ObjectiveStatus
	C             types.Destination
	SwapPrimitive SwapPrimitive
	SwapperIndex  uint
	Nonce         uint64
	StateSigs     map[uint]state.Signature
}

func (o Objective) MarshalJSON() ([]byte, error) {
	jsonSO := jsonObjective{
		Status:        o.Status,
		C:             o.C.Id,
		SwapPrimitive: o.SwapPrimitive,
		SwapperIndex:  o.SwapperIndex,
		StateSigs:     o.StateSigs,
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
	o.SwapperIndex = jsonSo.SwapperIndex
	o.SwapPrimitive = jsonSo.SwapPrimitive
	o.C = &channel.SwapChannel{}
	o.C.Id = jsonSo.C
	o.StateSigs = jsonSo.StateSigs

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
	sp, err := getSwapPayload(op.PayloadData)
	if err != nil {
		return Objective{}, fmt.Errorf("could not get swap primitive payload: %w", err)
	}

	obj, err := NewObjective(ObjectiveRequest{channelId: sp.SwapPrimitive.ChannelId}, preApprove, false, getChannelFunc, address)
	if err != nil {
		return Objective{}, fmt.Errorf("unable to construct swap objective from payload: %w", err)
	}

	obj.SwapPrimitive = sp.SwapPrimitive

	return obj, nil
}

func getSwapPayload(b []byte) (SwapPayload, error) {
	sp := SwapPayload{}
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
	fixedPart        state.FixedPart
	channelId        types.Destination
	nonce            uint64
	tokenIn          common.Address
	tokenOut         common.Address
	amountIn         *big.Int
	amountOut        *big.Int
	objectiveStarted chan struct{}
}

// NewObjectiveRequest creates a new ObjectiveRequest.
func NewObjectiveRequest(channelId types.Destination, tokenIn common.Address, tokenOut common.Address, amountIn *big.Int, amountOut *big.Int, fixedPart state.FixedPart, nonce uint64) ObjectiveRequest {
	return ObjectiveRequest{
		fixedPart:        fixedPart,
		nonce:            nonce,
		channelId:        channelId,
		tokenIn:          tokenIn,
		tokenOut:         tokenOut,
		amountIn:         amountIn,
		amountOut:        amountOut,
		objectiveStarted: make(chan struct{}),
	}
}

// Id returns the objective id for the request.
func (r ObjectiveRequest) Id(myAddress types.Address, chainId *big.Int) protocols.ObjectiveId {
	return protocols.ObjectiveId(ObjectivePrefix + getSwapPrimitiveId(r.fixedPart, r.nonce).String())
}

func getSwapPrimitiveId(fixedPart state.FixedPart, nonce uint64) types.Destination {
	fp := state.FixedPart{
		Participants:      fixedPart.Participants,
		ChannelNonce:      nonce,
		ChallengeDuration: fixedPart.ChallengeDuration,
		AppDefinition:     fixedPart.AppDefinition,
	}

	return fp.ChannelId()
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
		Id:        protocols.ObjectiveId(ObjectivePrefix + getSwapPrimitiveId(r.fixedPart, r.nonce).String()),
		ChannelId: r.channelId,
	}
}
