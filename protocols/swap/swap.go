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
	SwapPrimitivePayload protocols.PayloadType = "SwapPrimitivePayload"
)

const (
	WaitingForSwapping protocols.WaitingFor = "WaitingForSwapping"
	WaitingForNothing  protocols.WaitingFor = "WaitingForNothing" // Finished
)

const ObjectivePrefix = "Swap-"

// TODO: Discuss fields of exchange
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

// TODO: Create clone method for swap primitive

// TODO: Check need of custom marshall and unmarshall methods for swap primitive

// encodes the state into a []bytes value
func (sp SwapPrimitive) encode() (types.Bytes, error) {
	// TODO: Check whether we need to encode array of swap primitive
	// TODO: Check need of app data for sad path will be array of swap primitive
	return ethAbi.Arguments{
		{Type: abi.Destination}, // channel id
		{Type: abi.Address},     // fromAsset
		{Type: abi.Address},     // toAsset
		{Type: abi.Uint256},     // fromAmount
		{Type: abi.Uint256},     // toAmount
	}.Pack(
		sp.ChannelId,
		sp.Exchange.FromAsset,
		sp.Exchange.ToAsset,
		sp.Exchange.FromAmount,
		sp.Exchange.ToAmount,
	)
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
	// TODO: Validation
	sp.Sigs[myIndex] = sig
	return nil
}

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

	// fmt.Printf("SWAP OBJECTIVE>>>>>>>%+v", updated)
	// fmt.Printf("SWAP CHANNEL>>>>>>>%+v", updated.C)

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

		messages, err := protocols.CreateObjectivePayloadMessage(updated.Id(), updated.SwapPrimitive, SwapPrimitivePayload, o.C.Participants[1-o.C.MyIndex])
		if err != nil {
			return &updated, protocols.SideEffects{}, WaitingForSwapping, fmt.Errorf("could not create payload message %w", err)
		}

		fmt.Printf("SWAP updated.SwapPrimitive>>>>>>>%+v", updated.SwapPrimitive)

		sideEffects.MessagesToSend = append(sideEffects.MessagesToSend, messages...)
		return &updated, sideEffects, WaitingForSwapping, nil
	}

	// Wait if all signatures are not available
	if !updated.HasAllSignatures() {
		return &updated, protocols.SideEffects{}, WaitingForSwapping, nil
	}

	// If all signatures are available, update the swap channel according to the swap primitive and add the swap primitive to the channel.

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
	clone.IsSwapper = o.IsSwapper
	clone.C = o.C.Clone()
	// TODP: Create clone method for swap primitive
	clone.SwapPrimitive = o.SwapPrimitive

	return clone
}

// HasAllSignatures returns true if every participant has a valid signature.
func (o *Objective) HasAllSignatures() bool {
	// Since signatures are validated
	if len(o.SwapPrimitive.Sigs) == len(o.C.Participants) {
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

type jsonObjective struct {
	Status        protocols.ObjectiveStatus
	C             types.Destination
	SwapPrimitive SwapPrimitive
	IsSwapper     bool
}

func (o Objective) MarshalJSON() ([]byte, error) {
	jsonSO := jsonObjective{
		Status:        o.Status,
		C:             o.C.Id,
		SwapPrimitive: o.SwapPrimitive,
		IsSwapper:     o.IsSwapper,
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
	o.IsSwapper = jsonSo.IsSwapper
	o.SwapPrimitive = jsonSo.SwapPrimitive
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
