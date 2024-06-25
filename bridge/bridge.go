package bridge

import (
	"context"

	"github.com/statechannels/go-nitro/node"
	"github.com/statechannels/go-nitro/protocols"
)

type Bridge struct {
	nodeL1                      *node.Node
	nodeL2                      *node.Node
	completedObjectivesInNodeL1 <-chan protocols.ObjectiveId
	cancel                      context.CancelFunc
}

func New(nodeL1 *node.Node, nodeL2 *node.Node) Bridge {
	ctx, cancelFunc := context.WithCancel(context.Background())
	bridge := Bridge{
		nodeL1:                      nodeL1,
		nodeL2:                      nodeL2,
		completedObjectivesInNodeL1: nodeL1.CompletedObjectives(),
		cancel:                      cancelFunc,
	}

	go bridge.run(ctx)

	return bridge
}

func (b Bridge) run(ctx context.Context) {
	for {
		select {
		case objId := <-b.completedObjectivesInNodeL1:
			b.processObjectivesFromL1(objId)

		case <-ctx.Done():
			return
		}
	}
}

func (b Bridge) processObjectivesFromL1(objId protocols.ObjectiveId) {
	// TODO: If objectiveId corresponds to direct fund objective id
	// Create new outcome for mirrored ledger channel based on L1 ledger channel
	// Create mirrored ledger channel on L2 based on created outcome
}

func (b Bridge) Close() {
	b.cancel()
}
