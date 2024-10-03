// Package store contains the interface for a go-nitro store.
package store // import "github.com/statechannels/go-nitro/node/engine/store"

import (
	"io"
	"log/slog"
	"path/filepath"

	"github.com/statechannels/go-nitro/channel"
	"github.com/statechannels/go-nitro/channel/consensus_channel"
	"github.com/statechannels/go-nitro/crypto"
	"github.com/statechannels/go-nitro/payments"
	"github.com/statechannels/go-nitro/protocols"
	"github.com/statechannels/go-nitro/types"
	"github.com/tidwall/buntdb"
)

const (
	ErrNoSuchObjective  = types.ConstError("store: no such objective")
	ErrNoSuchChannel    = types.ConstError("store: failed to find required channel data")
	ErrNoSuchSwap       = types.ConstError("store: failed to find required swap data")
	ErrLoadVouchers     = types.ConstError("store: could not load vouchers")
	lastBlockNumSeenKey = "lastBlockNumSeen"
)

// Store is responsible for persisting objectives, objective metadata, states, signatures, private keys and blockchain data
type Store interface {
	GetChannelSecretKey() *[]byte                                                 // Get a pointer to a secret key for signing channel updates
	GetAddress() *types.Address                                                   // Get the (Ethereum) address associated with the ChannelSecretKey
	GetObjectiveById(protocols.ObjectiveId) (protocols.Objective, error)          // Read an existing objective
	GetObjectiveByChannelId(types.Destination) (obj protocols.Objective, ok bool) // Get the objective that currently owns the channel with the supplied ChannelId
	SetObjective(protocols.Objective) error                                       // Write an objective
	DestroyObjective(id protocols.ObjectiveId) error                              // Deletes an objective identified by the given ObjectiveId
	GetChannelsByIds(ids []types.Destination) ([]*channel.Channel, error)         // Returns a collection of channels with the given ids
	GetChannelById(id types.Destination) (c *channel.Channel, ok bool)
	GetChannelsByParticipant(participant types.Address) ([]*channel.Channel, error) // Returns any channels that includes the given participant
	GetAllChannels() ([]*channel.Channel, error)
	SetChannel(*channel.Channel) error
	DestroyChannel(id types.Destination) error
	GetChannelsByAppDefinition(appDef types.Address) ([]*channel.Channel, error) // Returns any channels that includes the given app definition
	ReleaseChannelFromOwnership(types.Destination) error                         // Release channel from being owned by any objective
	GetLastBlockNumSeen() (uint64, error)
	SetLastBlockNumSeen(uint64) error
	GetPendingSwapByChannelId(id types.Destination) (*payments.Swap, error)
	GetSwapById(id types.Destination) (payments.Swap, error)
	GetSwapsByChannelId(id types.Destination) ([]payments.Swap, error)
	SetChannelToSwaps(swap payments.Swap) error
	ConsensusChannelStore
	payments.VoucherStore
	io.Closer
}

type ConsensusChannelStore interface {
	GetAllConsensusChannels() ([]*consensus_channel.ConsensusChannel, error)
	GetConsensusChannel(counterparty types.Address) (channel *consensus_channel.ConsensusChannel, ok bool)
	GetConsensusChannelById(id types.Destination) (channel *consensus_channel.ConsensusChannel, err error)
	SetConsensusChannel(*consensus_channel.ConsensusChannel) error
	DestroyConsensusChannel(id types.Destination) error
}

type StoreOpts struct {
	PkBytes            []byte
	UseDurableStore    bool
	DurableStoreFolder string
	BuntDbConfig       buntdb.Config
}

func NewStore(options StoreOpts) (Store, error) {
	if options.PkBytes == nil {
		panic("pk must be provided to Store")
	}

	var ourStore Store
	var err error

	if options.UseDurableStore {
		me := crypto.GetAddressFromSecretKeyBytes(options.PkBytes)
		dataFolder := filepath.Join(options.DurableStoreFolder, me.String())

		slog.Info("Initialising durable store...", "dataFolder", dataFolder)
		ourStore, err = NewDurableStore(options.PkBytes, dataFolder, buntdb.Config{})
		if err != nil {
			return nil, err
		}
	} else {
		slog.Info("Initialising mem store...")
		ourStore = NewMemStore(options.PkBytes)
	}

	return ourStore, nil
}
