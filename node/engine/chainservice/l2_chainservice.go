package chainservice

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
	Bridge "github.com/statechannels/go-nitro/node/engine/chainservice/bridge"
	chainutils "github.com/statechannels/go-nitro/node/engine/chainservice/utils"
	"github.com/statechannels/go-nitro/protocols"
)

var (
	bridgeAbi, _       = Bridge.BridgeMetaData.GetAbi()
	statusUpdatedTopic = bridgeAbi.Events["StatusUpdated"].ID
)

var l2topicsToWatch = []common.Hash{
	statusUpdatedTopic,
}

type L2ChainService struct {
	*BaseChainService
	bridge        *Bridge.Bridge
	bridgeAddress common.Address
}

type L2ChainOpts struct {
	ChainUrl           string
	ChainStartBlockNum uint64
	ChainAuthToken     string
	ChainPk            string
	BridgeAddress      common.Address

	// Virtual payment and consensus app addresses are needed to be set in app definition of L2 state (required in L1 during challenge)
	VpaAddress common.Address
	CaAddress  common.Address
}

// NewL2ChainService is a convenient wrapper around newL2ChainService, which provides a simpler API
func NewL2ChainService(l2ChainOpts L2ChainOpts) (ChainService, error) {
	if l2ChainOpts.ChainPk == "" {
		return nil, fmt.Errorf("chainpk must be set")
	}

	ethClient, txSigner, err := chainutils.ConnectToChain(
		context.Background(),
		l2ChainOpts.ChainUrl,
		l2ChainOpts.ChainAuthToken,
		common.Hex2Bytes(l2ChainOpts.ChainPk),
	)
	if err != nil {
		panic(err)
	}

	bridge, err := Bridge.NewBridge(l2ChainOpts.BridgeAddress, ethClient)
	if err != nil {
		panic(err)
	}

	return newL2ChainService(ethClient, l2ChainOpts.ChainStartBlockNum, bridge, l2ChainOpts.BridgeAddress, l2ChainOpts.CaAddress, l2ChainOpts.VpaAddress, txSigner)
}

func newL2ChainService(chain ethChain, startBlockNum uint64, bridge *Bridge.Bridge,
	bridgeAddress, caAddress, vpaAddress common.Address, txSigner *bind.TransactOpts,
) (*L2ChainService, error) {
	baseCS, err := NewBaseChainService(chain, startBlockNum, txSigner, caAddress, vpaAddress)
	if err != nil {
		panic(err)
	}

	l2Cs := L2ChainService{
		BaseChainService: baseCS,
		bridge:           bridge,
		bridgeAddress:    bridgeAddress,
	}
	baseCS.DispatchChainEvents = l2Cs.DispatchChainEvents

	eventQuery := ethereum.FilterQuery{
		Addresses: []common.Address{l2Cs.bridgeAddress},
		Topics:    [][]common.Hash{l2topicsToWatch},
	}

	eventChan, err := l2Cs.SubscribeForLogs(eventQuery)
	if err != nil {
		return &L2ChainService{}, nil
	}

	l2Cs.Wg.Add(1)
	go l2Cs.ListenForEventLogs(eventChan, eventQuery)

	// Search for any missed events emitted while this node was offline
	err = l2Cs.CheckForMissedEvents(startBlockNum, eventQuery)
	if err != nil {
		return nil, err
	}

	return &l2Cs, nil
}

func (l2cs *L2ChainService) SendTransaction(tx protocols.ChainTransaction) error {
	switch tx := tx.(type) {
	case protocols.UpdateMirroredChannelStatesTransaction:
		_, err := l2cs.bridge.UpdateMirroredChannelStates(l2cs.defaultTxOpts(), tx.ChannelId(), tx.StateHash, tx.OutcomeBytes, tx.Amount, tx.Asset)
		return err
	default:
		return fmt.Errorf("unexpected transaction type %T", tx)
	}
}

// dispatchChainEvents takes in a collection of event logs from the chain
// and dispatches events to the out channel
func (l2cs *L2ChainService) DispatchChainEvents(logs []ethTypes.Log) error {
	for _, l := range logs {
		block, err := l2cs.chain.BlockByHash(context.Background(), l.BlockHash)
		if err != nil {
			return fmt.Errorf("error in getting block by hash %w", err)
		}

		switch l.Topics[0] {
		case statusUpdatedTopic:
			l2cs.logger.Debug("Processing StatusUpdated event")
			sue, err := l2cs.bridge.ParseStatusUpdated(l)
			if err != nil {
				return fmt.Errorf("error in ParseStatusUpdated: %w", err)
			}

			event := StatusUpdatedEvent{StateHash: sue.StateHash, commonEvent: commonEvent{channelID: sue.ChannelId, block: Block{BlockNum: l.BlockNumber, Timestamp: block.Time()}, txIndex: l.TxIndex}}
			l2cs.out <- event
		default:
			l2cs.logger.Info("Ignoring unknown chain event topic", "topic", l.Topics[0].String())

		}
	}
	return nil
}
