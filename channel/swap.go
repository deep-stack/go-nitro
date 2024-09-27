package channel

import (
	"errors"
	"fmt"
	"math/big"

	ethAbi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/statechannels/go-nitro/abi"
	"github.com/statechannels/go-nitro/channel/state"
	nc "github.com/statechannels/go-nitro/crypto"
	"github.com/statechannels/go-nitro/internal/queue"
	"github.com/statechannels/go-nitro/types"
)

const MaxSwapPrimitiveStorageLimit = 5

type SwapChannel struct {
	Channel

	SwapPrimitives queue.FixedQueue[SwapPrimitive]
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

	return &SwapChannel{*c, *queue.NewFixedQueue[SwapPrimitive](MaxSwapPrimitiveStorageLimit)}, err
}

// Clone returns a pointer to a new, deep copy of the receiver, or a nil pointer if the receiver is nil.
func (v *SwapChannel) Clone() *SwapChannel {
	if v == nil {
		return nil
	}

	// TODO: Add clone to swap primitives queue
	w := SwapChannel{*v.Channel.Clone(), v.SwapPrimitives}
	return &w
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

func (sp SwapPrimitive) Id() types.Destination {
	spHash, err := sp.Hash()
	if err != nil {
		return types.Destination{}
	}

	return types.Destination(spHash)
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

type Exchange struct {
	TokenIn   common.Address
	TokenOut  common.Address
	AmountIn  *big.Int
	AmountOut *big.Int
}

func (ex Exchange) Equal(target Exchange) bool {
	return ex.TokenIn == target.TokenIn && ex.TokenOut == target.TokenOut && ex.AmountIn.Cmp(target.AmountIn) == 0 && ex.AmountOut.Cmp(target.AmountOut) == 0
}
