package bridge

import (
	"context"
	"fmt"
	"log/slog"
	"math/big"
	"path/filepath"

	"github.com/ethereum/go-ethereum/common"
	"github.com/statechannels/go-nitro/channel/consensus_channel"
	"github.com/statechannels/go-nitro/channel/state"
	"github.com/statechannels/go-nitro/channel/state/outcome"
	nodeutils "github.com/statechannels/go-nitro/internal/node"
	"github.com/statechannels/go-nitro/internal/safesync"
	"github.com/statechannels/go-nitro/node"
	p2pms "github.com/statechannels/go-nitro/node/engine/messageservice/p2p-message-service"
	"github.com/statechannels/go-nitro/node/query"
	"github.com/statechannels/go-nitro/protocols/bridgeddefund"
	"github.com/statechannels/go-nitro/protocols/bridgedfund"
	"github.com/statechannels/go-nitro/protocols/directfund"
	"github.com/statechannels/go-nitro/protocols/virtualdefund"
	"github.com/tidwall/buntdb"

	"github.com/statechannels/go-nitro/node/engine/chainservice"
	"github.com/statechannels/go-nitro/node/engine/store"
	"github.com/statechannels/go-nitro/protocols"
	"github.com/statechannels/go-nitro/types"
)

const (
	L1_DURABLE_STORE_SUB_DIR = "l1-node"
	L2_DURABLE_STORE_SUB_DIR = "l2-node"
)

const RETRY_TX_LIMIT = 1

type Asset struct {
	L1AssetAddress string `toml:"l1AssetAddress"`
	L2AssetAddress string `toml:"l2AssetAddress"`
}

type L1ToL2AssetConfig struct {
	Assets []Asset `toml:"assets"`
}

type SentTx struct {
	Tx                  protocols.ChainTransaction `json:"tx"`
	NumOfRetries        uint                       `json:"num_of_retries"`
	IsRetryLimitReached bool                       `json:"is_retry_limit_reached"`
	IsL2                bool                       `json:"is_l2"`
}

type PendingTx struct {
	SentTx
	TxHash string `json:"tx_hash"`
}

type Bridge struct {
	bridgeStore *DurableStore

	nodeL1         *node.Node
	storeL1        store.Store
	chainServiceL1 chainservice.ChainService

	nodeL2         *node.Node
	storeL2        store.Store
	chainServiceL2 chainservice.ChainService

	cancel                context.CancelFunc
	L1ToL2AssetAddressMap map[common.Address]common.Address
	mirrorChannelMap      map[types.Destination]MirrorChannelDetails
	createdMirrorChannels chan types.Destination
	sentTxs               safesync.Map[SentTx]
}

type BridgeConfig struct {
	L1ChainUrl         string
	L2ChainUrl         string
	L1ChainStartBlock  uint64
	L2ChainStartBlock  uint64
	ChainPK            string
	StateChannelPK     string
	NaAddress          string
	VpaAddress         string
	CaAddress          string
	BridgeAddress      string
	Assets             []Asset
	DurableStoreDir    string
	BridgePublicIp     string
	NodeL1ExtMultiAddr string
	NodeL2ExtMultiAddr string
	NodeL1MsgPort      int
	NodeL2MsgPort      int
}

func New() *Bridge {
	bridge := Bridge{
		mirrorChannelMap:      make(map[types.Destination]MirrorChannelDetails),
		L1ToL2AssetAddressMap: make(map[common.Address]common.Address),
		createdMirrorChannels: make(chan types.Destination),
	}

	return &bridge
}

func (b *Bridge) Start(configOpts BridgeConfig) (nodeL1 *node.Node, nodeL2 *node.Node, nodeL1MultiAddress string, nodeL2MultiAddress string, err error) {
	chainOptsL1 := chainservice.ChainOpts{
		ChainUrl:   configOpts.L1ChainUrl,
		ChainPk:    configOpts.ChainPK,
		NaAddress:  common.HexToAddress(configOpts.NaAddress),
		VpaAddress: common.HexToAddress(configOpts.VpaAddress),
		CaAddress:  common.HexToAddress(configOpts.CaAddress),
	}

	chainOptsL2 := chainservice.L2ChainOpts{
		ChainUrl:      configOpts.L2ChainUrl,
		ChainPk:       configOpts.ChainPK,
		BridgeAddress: common.HexToAddress(configOpts.BridgeAddress),
		VpaAddress:    common.HexToAddress(configOpts.VpaAddress),
		CaAddress:     common.HexToAddress(configOpts.CaAddress),
	}

	storeOptsL1 := store.StoreOpts{
		PkBytes:            common.Hex2Bytes(configOpts.StateChannelPK),
		UseDurableStore:    true,
		DurableStoreFolder: filepath.Join(configOpts.DurableStoreDir, L1_DURABLE_STORE_SUB_DIR),
	}

	storeOptsL2 := store.StoreOpts{
		PkBytes:            common.Hex2Bytes(configOpts.StateChannelPK),
		UseDurableStore:    true,
		DurableStoreFolder: filepath.Join(configOpts.DurableStoreDir, L2_DURABLE_STORE_SUB_DIR),
	}

	messageOptsL1 := p2pms.MessageOpts{
		PkBytes:      common.Hex2Bytes(configOpts.StateChannelPK),
		TcpPort:      configOpts.NodeL1MsgPort,
		BootPeers:    nil,
		PublicIp:     configOpts.BridgePublicIp,
		ExtMultiAddr: configOpts.NodeL1ExtMultiAddr,
	}

	messageOptsL2 := p2pms.MessageOpts{
		PkBytes:      common.Hex2Bytes(configOpts.StateChannelPK),
		TcpPort:      configOpts.NodeL2MsgPort,
		BootPeers:    nil,
		PublicIp:     configOpts.BridgePublicIp,
		ExtMultiAddr: configOpts.NodeL2ExtMultiAddr,
	}

	// Initialize nodes
	nodeL1, storeL1, msgServiceL1, chainServiceL1, err := nodeutils.InitializeNode(chainOptsL1, storeOptsL1, messageOptsL1, &NodeL1PermissivePolicy{})
	if err != nil {
		return nil, nil, nodeL1MultiAddress, nodeL2MultiAddress, err
	}

	nodeL2, storeL2, msgServiceL2, chainServiceL2, err := nodeutils.InitializeL2Node(chainOptsL2, storeOptsL2, messageOptsL2)
	if err != nil {
		return nil, nil, nodeL1MultiAddress, nodeL2MultiAddress, err
	}

	// Process Assets array to convert it to map of L1 asset address to L2 asset address
	for _, asset := range configOpts.Assets {
		b.L1ToL2AssetAddressMap[common.HexToAddress(asset.L1AssetAddress)] = common.HexToAddress(asset.L2AssetAddress)
	}

	b.nodeL1 = nodeL1
	b.storeL1 = *storeL1
	b.chainServiceL1 = chainServiceL1
	b.nodeL2 = nodeL2
	b.storeL2 = *storeL2
	b.chainServiceL2 = chainServiceL2

	ctx, cancelFunc := context.WithCancel(context.Background())
	b.cancel = cancelFunc

	err = b.updateOnchainAssetAddressMap()
	if err != nil {
		return nil, nil, nodeL1MultiAddress, nodeL2MultiAddress, err
	}

	ds, err := NewDurableStore(configOpts.DurableStoreDir, buntdb.Config{})
	if err != nil {
		return nil, nil, nodeL1MultiAddress, nodeL2MultiAddress, err
	}

	b.bridgeStore = ds

	go b.listenForDroppedEvents(ctx)
	go b.run(ctx)

	return nodeL1, nodeL2, msgServiceL1.MultiAddr, msgServiceL2.MultiAddr, nil
}

func (b *Bridge) run(ctx context.Context) {
	completedObjectivesInNodeL1 := b.nodeL1.CompletedObjectives()
	completedObjectivesInNodeL2 := b.nodeL2.CompletedObjectives()

	for {
		var err error
		select {
		case objId, ok := <-completedObjectivesInNodeL1:
			if ok {
				err = b.processCompletedObjectivesFromL1(objId)
				b.checkError(err)
			}

		case objId, ok := <-completedObjectivesInNodeL2:
			if ok {
				err = b.processCompletedObjectivesFromL2(objId)
				b.checkError(err)
			}

		case <-ctx.Done():
			return
		}
	}
}

func (b *Bridge) processCompletedObjectivesFromL1(objId protocols.ObjectiveId) error {
	obj, err := b.storeL1.GetObjectiveById(objId)
	if err != nil {
		return fmt.Errorf("error in getting objective %w", err)
	}

	// If objectiveId corresponds to direct fund objective
	// Create new outcome for mirrored ledger channel based on L1 ledger channel
	// Create mirrored ledger channel on L2 based on created outcome
	ddFo, isDdfo := obj.(*directfund.Objective)
	if !isDdfo {
		return nil
	}

	channelId := ddFo.OwnsChannel()
	slog.Debug("Creating mirror outcome for L2", "channelId", channelId)
	l1LedgerChannel, err := b.storeL1.GetConsensusChannelById(channelId)
	if err != nil {
		return err
	}

	l1ledgerChannelState := l1LedgerChannel.SupportedSignedState()
	l1ledgerChannelStateClone := l1ledgerChannelState.Clone()

	// Put NodeBPrime's allocation at index 0 as it creates mirrored ledger channel
	// Swap the allocations to be set in mirrored ledger channel
	tempAllocation := l1ledgerChannelStateClone.State().Outcome[0].Allocations[0]
	l1ledgerChannelStateClone.State().Outcome[0].Allocations[0] = l1ledgerChannelStateClone.State().Outcome[0].Allocations[1]
	l1ledgerChannelStateClone.State().Outcome[0].Allocations[1] = tempAllocation

	// Create extended state outcome based on l1ChannelState
	l1ChannelCloneOutcome := l1ledgerChannelStateClone.State().Outcome
	var l2ChannelOutcome outcome.Exit

	for _, l1Outcome := range l1ChannelCloneOutcome {
		if (l1Outcome.Asset == common.Address{}) {
			l2ChannelOutcome = append(l2ChannelOutcome, l1Outcome)
		} else {
			value, ok := b.L1ToL2AssetAddressMap[l1Outcome.Asset]

			if !ok {
				return fmt.Errorf("could not find corresponding L2 asset address for L1 asset address %s", l1Outcome.Asset.String())
			}

			l1Outcome.Asset = value
			l2ChannelOutcome = append(l2ChannelOutcome, l1Outcome)
		}
	}

	// Create mirrored ledger channel between node BPrime and APrime
	l2LedgerChannelResponse, err := b.nodeL2.CreateBridgeChannel(l1ledgerChannelStateClone.State().Participants[0], l1ledgerChannelStateClone.State().ChallengeDuration, l2ChannelOutcome)
	if err != nil {
		return err
	}

	err = b.bridgeStore.SetMirrorChannelDetails(l2LedgerChannelResponse.ChannelId, MirrorChannelDetails{L1ChannelId: l1LedgerChannel.Id})
	if err != nil {
		return err
	}

	slog.Debug("Started creating mirror ledger channel in L2", "channelId", l2LedgerChannelResponse.ChannelId)
	return nil
}

func (b *Bridge) processCompletedObjectivesFromL2(objId protocols.ObjectiveId) error {
	obj, err := b.storeL2.GetObjectiveById(objId)
	if err != nil {
		return fmt.Errorf("error in getting objective %w", err)
	}

	switch objective := obj.(type) {

	case *bridgedfund.Objective:
		l2channelId := objective.OwnsChannel()

		mirrorChannelDetails, err := b.bridgeStore.GetMirrorChannelDetails(l2channelId)
		if err != nil {
			return err
		}

		err = b.bridgeStore.SetMirrorChannelDetails(l2channelId, MirrorChannelDetails{L1ChannelId: mirrorChannelDetails.L1ChannelId, IsCreated: true})
		if err != nil {
			return err
		}

		// Node B calls contract method to store L2ChannelId => L1ChannelId
		setL2ToL1TxToSubmit := protocols.NewSetL2ToL1Transaction(mirrorChannelDetails.L1ChannelId, l2channelId)
		setL2ToL1Tx, err := b.chainServiceL1.SendTransaction(setL2ToL1TxToSubmit)
		if err != nil {
			return fmt.Errorf("error in send transaction %w", err)
		}

		b.sentTxs.Store(setL2ToL1Tx.Hash().String(), SentTx{setL2ToL1TxToSubmit, 0, false, false})

		// use a nonblocking send in case no one is listening
		select {
		case b.createdMirrorChannels <- l2channelId:
		default:
		}

	case *virtualdefund.Objective:
		// Get ledger channels from virtual defund objective
		var ledgerChannels []*consensus_channel.ConsensusChannel
		if objective.ToMyLeft != nil {
			ledgerChannels = append(ledgerChannels, objective.ToMyLeft)
		}

		if objective.ToMyRight != nil {
			ledgerChannels = append(ledgerChannels, objective.ToMyRight)
		}

		// Updates the bridge contract with the latest state of ledger channels
		for _, ch := range ledgerChannels {
			txToSubmit, err := b.getUpdateMirrorChannelStateTransaction(ch)
			if err != nil {
				return err
			}

			tx, err := b.chainServiceL2.SendTransaction(txToSubmit)
			if err != nil {
				return fmt.Errorf("error in send transaction %w", err)
			}

			b.sentTxs.Store(tx.Hash().String(), SentTx{txToSubmit, 0, false, true})
		}

	case *bridgeddefund.Objective:
		// Get latest supported signed state of L2
		signedState, err := objective.C.LatestSupportedSignedState()
		if err != nil {
			return fmt.Errorf("error in latest supported signed state: %w", err)
		}

		// Get L1 ledger channel Id
		mirrorInfo, err := b.bridgeStore.GetMirrorChannelDetails(obj.OwnsChannel())
		if err != nil {
			return fmt.Errorf("error in getting mirror channel details: %w", err)
		}

		// Initiate mirror bridged defund on L1 using L2 signed state
		_, err = b.nodeL1.MirrorBridgedDefund(mirrorInfo.L1ChannelId, signedState, false)
		if err != nil {
			return fmt.Errorf("error in initiating mirror bridged defund: %w", err)
		}
	}

	return nil
}

// Get update mirror channel state transaction from given consensus channel
func (b *Bridge) getUpdateMirrorChannelStateTransaction(con *consensus_channel.ConsensusChannel) (protocols.ChainTransaction, error) {
	// Get latest outcome bytes
	ledgerOutcome := con.ConsensusVars().Outcome
	outcome := ledgerOutcome.AsOutcome()
	outcomeByte, err := outcome.Encode()
	if err != nil {
		return nil, err
	}

	// Get latest state hash
	state := con.ConsensusVars().AsState(con.FixedPart())
	stateHash, err := state.Hash()
	if err != nil {
		return nil, err
	}

	asset := outcome[0].Asset
	// Calculate latest holdings
	holdingAmount := new(big.Int)
	for _, allocation := range outcome[0].Allocations {
		holdingAmount.Add(holdingAmount, allocation.Amount)
	}

	updateMirroredChannelStateTx := protocols.NewUpdateMirroredChannelStatesTransaction(con.Id, stateHash, outcomeByte, asset, holdingAmount)

	return updateMirroredChannelStateTx, nil
}

// Set L2AssetAddress => L1AssetAddress if it doesn't already exist on L1 chain
func (b *Bridge) updateOnchainAssetAddressMap() error {
	l1AssetAddressesTxSent := make(map[common.Address]struct{})

	for l1AssetAddress, l2AssetAddress := range b.L1ToL2AssetAddressMap {
		l1OnchainAssetAddress, err := b.chainServiceL1.GetL1AssetAddressFromL2(l2AssetAddress)
		if err != nil {
			return err
		}

		if l1OnchainAssetAddress != l1AssetAddress {
			setL2ToL1AssetAddressTxToSubmit := protocols.NewSetL2ToL1AssetAddressTransaction(l1AssetAddress, l2AssetAddress)
			_, err := b.chainServiceL1.SendTransaction(setL2ToL1AssetAddressTxToSubmit)
			if err != nil {
				return fmt.Errorf("failed to send transaction for updating asset address mapping: %w", err)
			}
			l1AssetAddressesTxSent[l1AssetAddress] = struct{}{}
		}
	}

	if len(l1AssetAddressesTxSent) > 0 {
		for event := range b.chainServiceL1.EventFeed() {
			assetMapUpdatedEvent, ok := event.(chainservice.AssetMapUpdatedEvent)
			if !ok {
				continue
			}

			_, ok = l1AssetAddressesTxSent[assetMapUpdatedEvent.L1AssetAddress]
			if ok {
				delete(l1AssetAddressesTxSent, assetMapUpdatedEvent.L1AssetAddress)
			}

			if len(l1AssetAddressesTxSent) == 0 {
				break
			}
		}
	}

	return nil
}

// Since bridge node addresses are same
func (b *Bridge) GetBridgeAddress() common.Address {
	return *b.nodeL1.Address
}

func (b *Bridge) GetL2SupportedSignedState(id types.Destination) (state.SignedState, error) {
	return b.nodeL2.GetSignedState(id)
}

func (b *Bridge) MirrorBridgedDefund(l1ChannelId types.Destination, l2SignedState state.SignedState, isChallenge bool) (protocols.ObjectiveId, error) {
	return b.nodeL1.MirrorBridgedDefund(l1ChannelId, l2SignedState, isChallenge)
}

func (b *Bridge) CounterChallenge(id types.Destination, action types.CounterChallengeAction, payload state.SignedState) {
	b.nodeL1.CounterChallenge(id, action, payload)
}

func (b *Bridge) GetL2ChannelIdByL1ChannelId(l1ChannelId types.Destination) (l2ChannelId types.Destination, isCreated bool) {
	var err error
	l2ChannelId, isCreated, err = b.bridgeStore.GetMirrorChannelDetailsByL1Channel(l1ChannelId)
	if err != nil {
		return l2ChannelId, isCreated
	}

	return l2ChannelId, isCreated
}

func (b *Bridge) GetL2ObjectiveByL1ObjectiveId(l1ObjectiveId protocols.ObjectiveId) (protocols.Objective, error) {
	l1Objective, err := b.storeL1.GetObjectiveById(l1ObjectiveId)
	if err != nil {
		return nil, err
	}

	l1ChannelId := l1Objective.OwnsChannel()
	l2ChannelId, _ := b.GetL2ChannelIdByL1ChannelId(l1ChannelId)

	if l2ChannelId.IsZero() {
		return nil, fmt.Errorf("could not find L2 channel for given L1 objective ID")
	}

	l2Objective, ok := b.storeL2.GetObjectiveByChannelId(l2ChannelId)
	if !ok {
		return nil, fmt.Errorf("corresponding L2 objective is either complete or does not exist")
	}

	return l2Objective, nil
}

func (b *Bridge) GetObjectiveById(objectiveId protocols.ObjectiveId, l2 bool) (protocols.Objective, error) {
	if l2 {
		return b.nodeL2.GetObjectiveById(objectiveId)
	}
	return b.nodeL1.GetObjectiveById(objectiveId)
}

func (b *Bridge) GetAllL2Channels() ([]query.LedgerChannelInfo, error) {
	return b.nodeL2.GetAllLedgerChannels()
}

func (b *Bridge) CreatedMirrorChannels() <-chan types.Destination {
	return b.createdMirrorChannels
}

func (b *Bridge) RetryObjectiveTx(objectiveId protocols.ObjectiveId) error {
	if bridgedfund.IsBridgedFundObjective(objectiveId) {
		b.nodeL2.RetryObjectiveTx(objectiveId)
		return nil
	}
	if directfund.IsDirectFundObjective(objectiveId) {
		b.nodeL1.RetryObjectiveTx(objectiveId)
		return nil
	}
	return fmt.Errorf("objective with given Id is not supported for retrying")
}

func (b *Bridge) RetryTx(txHash common.Hash) error {
	var chainService chainservice.ChainService

	txToRetry, ok := b.sentTxs.Load(txHash.String())
	if !ok {
		return fmt.Errorf("tx with given hash %s was either complete or cannot be found", txHash)
	}

	if !txToRetry.IsRetryLimitReached {
		return fmt.Errorf("tx with given hash %s is pending confirmation and connot be retried", txHash)
	}

	if txToRetry.IsL2 {
		chainService = b.chainServiceL2
	} else {
		chainService = b.chainServiceL1
	}

	_, err := chainService.SendTransaction(txToRetry.Tx)
	if err != nil {
		return err
	}

	return nil
}

func (b *Bridge) Close() error {
	b.cancel()
	err := b.nodeL1.Close()
	if err != nil {
		return err
	}

	err = b.nodeL2.Close()
	if err != nil {
		return err
	}

	return b.bridgeStore.Close()
}

func (b *Bridge) checkError(err error) {
	if err != nil {
		slog.Error("error in run loop", "error", err)
	}
}

func (b *Bridge) listenForDroppedEvents(ctx context.Context) {
	var err error
	for {
		select {
		case l1DroppedEvent := <-b.chainServiceL1.DroppedEventFeed():
			err = b.checkAndRetryDroppedTxs(l1DroppedEvent, b.chainServiceL1, false)

		case l2DroppedEvent := <-b.chainServiceL2.DroppedEventFeed():
			err = b.checkAndRetryDroppedTxs(l2DroppedEvent, b.chainServiceL2, true)

		case l1ConfirmedEvent := <-b.chainServiceL1.EventFeed():
			b.sentTxs.Delete(l1ConfirmedEvent.TxHash().String())

		case l2ConfirmedEvent := <-b.chainServiceL2.EventFeed():
			b.sentTxs.Delete(l2ConfirmedEvent.TxHash().String())

		case <-ctx.Done():
			return
		}

		b.checkError(err)
	}
}

func (b *Bridge) checkAndRetryDroppedTxs(droppedEvent protocols.DroppedEventInfo, chainservice chainservice.ChainService, isL2 bool) error {
	txToRetry, ok := b.sentTxs.Load(droppedEvent.TxHash.String())
	if !ok {
		return nil
	}

	if txToRetry.NumOfRetries >= RETRY_TX_LIMIT {
		txToRetry.IsRetryLimitReached = true
		b.sentTxs.Store(droppedEvent.TxHash.String(), txToRetry)
		return nil
	}

	retriedTx, err := chainservice.SendTransaction(txToRetry.Tx)
	if err != nil {
		return err
	}

	b.sentTxs.Delete(droppedEvent.TxHash.String())
	b.sentTxs.Store(retriedTx.Hash().String(), SentTx{txToRetry.Tx, txToRetry.NumOfRetries + 1, false, isL2})
	return nil
}

func (b *Bridge) GetPendingBridgeTxs(channelId types.Destination) []PendingTx {
	var foundPendingTx []PendingTx
	b.sentTxs.Range(func(txHash string, sentTxInfo SentTx) bool {
		if sentTxInfo.Tx.ChannelId() == channelId {
			foundPendingTx = append(foundPendingTx, PendingTx{sentTxInfo, txHash})
		}
		return true
	})

	return foundPendingTx
}

func (b *Bridge) GetNodeInfo() types.NodeInfo {
	// State channel address and message service peer ID for both the L1 and L2 nodes are same because the same private key is being used for both nodes
	return b.nodeL1.GetNodeInfo()
}
