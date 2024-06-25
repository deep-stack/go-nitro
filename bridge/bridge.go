package bridge

import (
	"context"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/statechannels/go-nitro/node"
	"github.com/statechannels/go-nitro/node/engine/store"
	"github.com/statechannels/go-nitro/protocols"
	"github.com/statechannels/go-nitro/types"
)

type Bridge struct {
	nodeL1                      *node.Node
	nodeL2                      *node.Node
	storeL1                     store.Store
	storeL2                     store.Store
	completedObjectivesInNodeL1 <-chan protocols.ObjectiveId
	cancel                      context.CancelFunc
}

func New(nodeL1 *node.Node, nodeL2 *node.Node, storeL1 *store.Store, storeL2 *store.Store) Bridge {
	ctx, cancelFunc := context.WithCancel(context.Background())
	bridge := Bridge{
		nodeL1:                      nodeL1,
		nodeL2:                      nodeL2,
		storeL1:                     *storeL1,
		storeL2:                     *storeL2,
		completedObjectivesInNodeL1: nodeL1.CompletedObjectives(),
		cancel:                      cancelFunc,
	}

	go bridge.run(ctx)

	return bridge
}

func (b Bridge) run(ctx context.Context) {
	for {
		var err error
		select {
		case objId := <-b.completedObjectivesInNodeL1:
			err = b.processObjectivesFromL1(objId)
			b.checkError(err)
		case <-ctx.Done():
			return
		}
	}
}

func (b Bridge) processObjectivesFromL1(objId protocols.ObjectiveId) error {
	objIdArr := strings.Split(string(objId), "-")
	objectiveType := objIdArr[0]
	channelId := objIdArr[1]

	// If objectiveId corresponds to direct fund objective
	// Create new outcome for mirrored ledger channel based on L1 ledger channel
	// Create mirrored ledger channel on L2 based on created outcome
	if objectiveType == "DirectFunding" {
		fmt.Println("Creating mirror outcome for L2", channelId)
		l1LedgerChannel, err := b.storeL1.GetConsensusChannelById(types.Destination(common.HexToHash(channelId)))
		if err != nil {
			return err
		}
		l1ledgerChannelState := l1LedgerChannel.SupportedSignedState()
		l1ledgerChannelStateClone := l1ledgerChannelState.Clone()

		// Swap the destination
		tempDestination := l1ledgerChannelStateClone.State().Outcome[0].Allocations[0].Destination
		l1ledgerChannelStateClone.State().Outcome[0].Allocations[0].Destination = l1ledgerChannelStateClone.State().Outcome[0].Allocations[1].Destination
		l1ledgerChannelStateClone.State().Outcome[0].Allocations[1].Destination = tempDestination

		// Create extended state outcome based on l1ChannelState
		l2ChannelOutcome := l1ledgerChannelStateClone.State().Outcome

		// Create mirrored ledger channel between node BPrime and APrime
		response, err := b.nodeL2.CreateBridgeChannel(l1ledgerChannelState.State().Participants[0], uint32(10), l2ChannelOutcome)
		if err != nil {
			return err
		}
		fmt.Println("Started creating mirror ledger channel in L2", response.ChannelId)
	}

	return nil
}

func (b Bridge) Close() {
	b.cancel()
}

func (b Bridge) checkError(err error) {
	if err != nil {
		fmt.Println("error in run loop", err)
		panic(err)
	}
}
