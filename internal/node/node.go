package node

import (
	"log/slog"

	"github.com/statechannels/go-nitro/node"
	"github.com/statechannels/go-nitro/node/engine"
	"github.com/statechannels/go-nitro/node/engine/chainservice"
	"github.com/statechannels/go-nitro/node/engine/store"

	p2pms "github.com/statechannels/go-nitro/node/engine/messageservice/p2p-message-service"
)

func InitializeNode(chainOpts chainservice.ChainOpts, storeOpts store.StoreOpts, messageOpts p2pms.MessageOpts, policymaker engine.PolicyMaker) (*node.Node, *store.Store, *p2pms.P2PMessageService, chainservice.ChainService, error) {
	ourStore, err := store.NewStore(storeOpts)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	slog.Info("Initializing message service", "tcp port", messageOpts.TcpPort, "web socket port", messageOpts.WsMsgPort)
	messageOpts.SCAddr = *ourStore.GetAddress()
	messageService := p2pms.NewMessageService(messageOpts)

	storeBlockNum, err := ourStore.GetLastBlockNumSeen()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	chainOpts.ChainStartBlockNum = storeBlockNum

	slog.Info("Initializing chain service...")
	ourChain, err := chainservice.NewEthChainService(chainOpts)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	node := node.New(
		messageService,
		ourChain,
		ourStore,
		policymaker,
	)

	return &node, &ourStore, messageService, ourChain, nil
}
