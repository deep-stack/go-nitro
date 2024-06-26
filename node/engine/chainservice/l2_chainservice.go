package chainservice

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"

	Bridge "github.com/statechannels/go-nitro/node/engine/chainservice/bridge"
	chainutils "github.com/statechannels/go-nitro/node/engine/chainservice/utils"
	"github.com/statechannels/go-nitro/protocols"
)

type L2ChainOpts struct {
	ChainUrl           string
	ChainStartBlockNum uint64
	ChainAuthToken     string
	ChainPk            string
	BridgeAddress      common.Address
}

type L2ChainService struct {
	*EthChainService
	bridge        *Bridge.Bridge
	bridgeAddress common.Address
}

// NewL2ChainService is a convenient wrapper around newL2ChainService, which provides a simpler API
func NewL2ChainService(l2ChainOpts L2ChainOpts) (*L2ChainService, error) {
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

	na, err := Bridge.NewBridge(l2ChainOpts.BridgeAddress, ethClient)
	if err != nil {
		panic(err)
	}

	return newL2ChainService(ethClient, l2ChainOpts.ChainStartBlockNum, na, l2ChainOpts.BridgeAddress, txSigner)
}

// newL2ChainService constructs a chain service that submits transactions to a Bridge contract
// and listens to events from an eventSource
func newL2ChainService(chain ethChain, startBlockNum uint64, bridge *Bridge.Bridge,
	bridgeAddress common.Address, txSigner *bind.TransactOpts,
) (*L2ChainService, error) {
	ecs, err := newEthChainService(chain, startBlockNum, nil, common.Address{}, common.Address{}, common.Address{}, txSigner)
	if err != nil {
		return nil, err
	}

	// Use a buffered channel so we don't have to worry about blocking on writing to the channel.
	l2cs := L2ChainService{ecs, bridge, bridgeAddress}
	return &l2cs, nil
}

func (l2cs *L2ChainService) SendTransaction(tx protocols.ChainTransaction) error {
	switch tx := tx.(type) {
	case protocols.UpdateMirroredChannelStatusTransaction:
		_, err := l2cs.bridge.UpdateMirroredChannelStatus(l2cs.defaultTxOpts(), tx.ChannelId(), tx.StateHash, tx.OutcomeBytes)
		return err
	case protocols.GetMirroredChannelStatusTransaction:
		_, err := l2cs.bridge.GetMirroredChannelStatus(&bind.CallOpts{}, tx.ChannelId())
		return err
	default:
		return fmt.Errorf("unexpected transaction type %T", tx)
	}
}
