package bridge

import (
	"github.com/statechannels/go-nitro/node"
)

type Bridge struct {
	node      *node.Node
	nodePrime *node.Node
}

func New(node *node.Node, nodePrime *node.Node) Bridge {
	return Bridge{
		node:      node,
		nodePrime: nodePrime,
	}
}
