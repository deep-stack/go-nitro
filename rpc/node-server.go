package rpc

import (
	nitro "github.com/statechannels/go-nitro/node"
	"github.com/statechannels/go-nitro/rpc/transport"
)

type NodeRpcServer struct {
	*BaseRpcServer
	node *nitro.Node
}

func NewNodeRpcServer(node *nitro.Node, trans transport.Responder) (*NodeRpcServer, error) {
	baseRpcServer := NewBaseRpcServer(trans)

	nodeRpcServer := &NodeRpcServer{
		baseRpcServer,
		node,
	}

	err := nodeRpcServer.registerHandlers()
	if err != nil {
		return nil, err
	}

	return nodeRpcServer, nil
}

func (rs *NodeRpcServer) registerHandlers() (err error) {
	return nil
}
