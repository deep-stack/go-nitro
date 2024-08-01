package chainservice

import (
	"context"
	"fmt"
	"log/slog"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	ethTypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/statechannels/go-nitro/internal/logging"
	"github.com/statechannels/go-nitro/types"
)

type ChainOpts struct {
	ChainUrl           string
	ChainStartBlockNum uint64
	ChainAuthToken     string
	ChainPk            string
	NaAddress          common.Address
	VpaAddress         common.Address
	CaAddress          common.Address
}

const (
	MIN_BACKOFF_TIME = 1 * time.Second
	MAX_BACKOFF_TIME = 5 * time.Minute
)

type ethChain interface {
	bind.ContractBackend
	ethereum.TransactionReader
	ethereum.ChainReader
	ChainID(ctx context.Context) (*big.Int, error)
	TransactionSender(ctx context.Context, tx *ethTypes.Transaction, block common.Hash, index uint) (common.Address, error)
}

// eventTracker holds on to events in memory and dispatches an event after required number of confirmations
type BaseChainService struct {
	chain                    ethChain
	txSigner                 *bind.TransactOpts
	out                      chan Event
	logger                   *slog.Logger
	ctx                      context.Context
	cancel                   context.CancelFunc
	Wg                       *sync.WaitGroup
	eventTracker             *eventTracker
	eventSub                 ethereum.Subscription
	newBlockSub              ethereum.Subscription
	newBlockChan             chan *ethTypes.Header
	consensusAppAddress      common.Address
	virtualPaymentAppAddress common.Address
	errorChan                chan error
	DispatchChainEvents      func(logs []ethTypes.Log) error
}

// MAX_QUERY_BLOCK_RANGE is the maximum range of blocks we query for events at once.
// Most json-rpc nodes restrict the amount of blocks you can search.
// For example Wallaby supports a maximum range of 2880
// See https://github.com/Zondax/rosetta-filecoin/blob/b395b3e04401be26c6cdf6a419e14ce85e2f7331/tools/wallaby/files/config.toml#L243
const MAX_QUERY_BLOCK_RANGE = 2000

// RESUB_INTERVAL is how often we resubscribe to log events.
// We do this to avoid https://github.com/ethereum/go-ethereum/issues/23845
// We use 2.5 minutes as the default filter timeout is 5 minutes.
// See https://github.com/ethereum/go-ethereum/blob/e14164d516600e9ac66f9060892e078f5c076229/eth/filters/filter_system.go#L43
// This has been reduced to 15 seconds to support local devnets with much shorter timeouts.
const RESUB_INTERVAL = 15 * time.Second

// REQUIRED_BLOCK_CONFIRMATIONS is how many blocks must be mined before an emitted event is processed
const REQUIRED_BLOCK_CONFIRMATIONS = 2

// MAX_EPOCHS is the maximum range of old epochs we can query with a single "FilterLogs" request
// This is a restriction enforced by the rpc provider
const MAX_EPOCHS = 60480

// BLOCKS_WITHOUT_EVENT_THRESHOLD is the maximum number of blocks the node will wait for an event to confirm transaction to be mined
const BLOCKS_WITHOUT_EVENT_THRESHOLD = 16

// GAS_LIMIT_MULTIPLIER is the multiplier for updating gas limit
const GAS_LIMIT_MULTIPLIER = 1.5

// NewBaseChainService constructs a chain service that submits transactions to a NitroAdjudicator
// and listens to events from an eventSource
func NewBaseChainService(chain ethChain, startBlockNum uint64, txSigner *bind.TransactOpts, caAddress, vpaAddress common.Address) (*BaseChainService, error) {
	ctx, cancelCtx := context.WithCancel(context.Background())

	logger := logging.LoggerWithAddress(slog.Default(), txSigner.From)

	block, err := chain.BlockByNumber(ctx, new(big.Int).SetUint64(startBlockNum))
	if err != nil {
		cancelCtx()
		return nil, err
	}
	startBlock := Block{
		BlockNum:  block.NumberU64(),
		Timestamp: block.Time(),
	}
	tracker := NewEventTracker(startBlock)

	// Use a buffered channel so we don't have to worry about blocking on writing to the channel.
	ecs := BaseChainService{
		chain:                    chain,
		txSigner:                 txSigner,
		out:                      make(chan Event, 10),
		logger:                   logger,
		ctx:                      ctx,
		cancel:                   cancelCtx,
		Wg:                       &sync.WaitGroup{},
		eventTracker:             tracker,
		consensusAppAddress:      caAddress,
		virtualPaymentAppAddress: caAddress,
		errorChan:                make(chan error),
	}

	newBlockChan, err := ecs.subscribeNewHead()
	if err != nil {
		return nil, err
	}
	ecs.newBlockChan = newBlockChan

	// Prevent go routines from processing events before checkForMissedEvents completes
	ecs.eventTracker.mu.Lock()
	defer ecs.eventTracker.mu.Unlock()

	ecs.Wg.Add(2)
	go ecs.listenForNewBlocks(newBlockChan)
	go ecs.listenForErrors()

	return &ecs, nil
}

func (bcs *BaseChainService) CheckForMissedEvents(startBlock uint64, eventQuery ethereum.FilterQuery) error {
	// Fetch the latest block
	latestBlock, err := bcs.chain.BlockByNumber(bcs.ctx, nil)
	if err != nil {
		return err
	}

	latestBlockNum := latestBlock.NumberU64()
	bcs.logger.Info("checking for missed chain events", "startBlock", startBlock, "currentBlock", latestBlockNum)

	// Loop through in chunks of MAX_EPOCHS
	for currentStart := startBlock; currentStart <= latestBlockNum; {
		currentEnd := currentStart + MAX_EPOCHS
		if currentEnd > latestBlockNum {
			currentEnd = latestBlockNum
		}

		// Update event query for the current chunk
		eventQuery.FromBlock = big.NewInt(int64(currentStart))
		eventQuery.ToBlock = big.NewInt(int64(currentEnd))

		// Fetch logs for the current chunk
		missedEvents, err := bcs.chain.FilterLogs(bcs.ctx, eventQuery)
		if err != nil {
			bcs.logger.Error("failed to retrieve old chain logs. " + err.Error())
			errorMsg := "*** To avoid this error, consider increasing the chainstartblock value in your configuration before restarting the node."
			errorMsg += " Note that this may cause your node to miss chain events emitted prior to the chainstartblock."
			bcs.logger.Error(errorMsg)
			return err
		}
		bcs.logger.Info("finished checking for missed chain events in range", "fromBlock", currentStart, "toBlock", currentEnd, "numMissedEvents", len(missedEvents))

		for _, event := range missedEvents {
			bcs.eventTracker.Push(event)
		}

		currentStart = currentEnd + 1 // Move to the next chunk
	}

	return nil
}

// listenForErrors listens for errors on the error channel and attempts to handle them if they occur.
// TODO: Currently "handle" is panicking
func (bcs *BaseChainService) listenForErrors() {
	for {
		select {
		case <-bcs.ctx.Done():
			bcs.Wg.Done()
			return
		case err := <-bcs.errorChan:
			bcs.logger.Error("chain service error", "error", err)
			panic(err)
		}
	}
}

// defaultTxOpts returns transaction options suitable for most transaction submissions
func (bcs *BaseChainService) defaultTxOpts() *bind.TransactOpts {
	return &bind.TransactOpts{
		From:      bcs.txSigner.From,
		Nonce:     bcs.txSigner.Nonce,
		Signer:    bcs.txSigner.Signer,
		GasFeeCap: bcs.txSigner.GasFeeCap,
		GasTipCap: bcs.txSigner.GasTipCap,
		GasLimit:  bcs.txSigner.GasLimit,
		GasPrice:  bcs.txSigner.GasPrice,
	}
}

func (bcs *BaseChainService) ListenForEventLogs(eventChan chan ethTypes.Log, eventQuery ethereum.FilterQuery) {
	for {
		select {
		case <-bcs.ctx.Done():
			bcs.eventSub.Unsubscribe()
			bcs.Wg.Done()
			return

		case err := <-bcs.eventSub.Err():
			latestBlockNum := bcs.GetLastConfirmedBlockNum()

			if err != nil {
				bcs.logger.Warn("error in chain event subscription: " + err.Error())
				bcs.eventSub.Unsubscribe()
			} else {
				bcs.logger.Warn("chain event subscription closed")
			}

			resubscribed := false // Flag to indicate whether resubscription was successful

			// Use exponential backoff loop to attempt to re-establish subscription
			for backoffTime := MIN_BACKOFF_TIME; backoffTime < MAX_BACKOFF_TIME; backoffTime *= 2 {
				select {
				// Exit from resubscription loop on closing chain service (cancelling context)
				// https://github.com/golang/go/issues/39483
				case <-time.After(backoffTime):
					eventSub, err := bcs.chain.SubscribeFilterLogs(bcs.ctx, eventQuery, eventChan)
					if err != nil {
						bcs.logger.Warn("failed to resubscribe to chain events, retrying", "backoffTime", backoffTime)
						continue
					}

					bcs.eventSub = eventSub
					bcs.logger.Debug("resubscribed to chain events")

					bcs.eventTracker.mu.Lock()
					err = bcs.CheckForMissedEvents(latestBlockNum, eventQuery)
					bcs.eventTracker.mu.Unlock()

					if err != nil {
						bcs.errorChan <- fmt.Errorf("subscribeFilterLogs failed during checkForMissedEvents: " + err.Error())
						return
					}

					resubscribed = true

				case <-bcs.ctx.Done():
					bcs.Wg.Done()
					bcs.eventSub.Unsubscribe()
					return
				}

				if resubscribed {
					break
				}
			}

			if !resubscribed {
				bcs.logger.Error("subscribeFilterLogs failed to resubscribe")
				bcs.errorChan <- fmt.Errorf("subscribeFilterLogs failed to resubscribe")
				return
			}

		case <-time.After(RESUB_INTERVAL):
			// Due to https://github.com/ethereum/go-ethereum/issues/23845 we can't rely on a long running subscription.
			// We unsub here and recreate the subscription in the next iteration of the select.
			bcs.eventSub.Unsubscribe()

		case chainEvent := <-eventChan:
			bcs.logger.Debug("queueing new chainEvent", "block-num", chainEvent.BlockNumber)
			bcs.updateEventTracker(nil, &chainEvent)
		}
	}
}

func (bcs *BaseChainService) listenForNewBlocks(newBlockChan chan *ethTypes.Header) {
	for {
		select {
		case <-bcs.ctx.Done():
			bcs.newBlockSub.Unsubscribe()
			bcs.Wg.Done()
			return

		case err := <-bcs.newBlockSub.Err():
			if err != nil {
				bcs.logger.Warn("error in chain new block subscription: " + err.Error())
				bcs.newBlockSub.Unsubscribe()
			} else {
				bcs.logger.Warn("chain new block subscription closed")
			}

			// Use exponential backoff loop to attempt to re-establish subscription
			resubscribed := false // Flag to indicate whether resubscription was successful

			for backoffTime := MIN_BACKOFF_TIME; backoffTime < MAX_BACKOFF_TIME; backoffTime *= 2 {
				select {
				// Exit from resubscription loop on closing chain service (cancelling context)
				// https://github.com/golang/go/issues/39483
				case <-time.After(backoffTime):
					newBlockSub, err := bcs.chain.SubscribeNewHead(bcs.ctx, newBlockChan)
					if err != nil {
						bcs.logger.Warn("subscribeNewHead failed to resubscribe: " + err.Error())
						continue
					}

					bcs.newBlockSub = newBlockSub
					bcs.logger.Debug("resubscribed to chain new blocks")
					resubscribed = true

				case <-bcs.ctx.Done():
					bcs.newBlockSub.Unsubscribe()
					bcs.Wg.Done()
					return
				}

				if resubscribed {
					break
				}
			}

			if !resubscribed {
				bcs.errorChan <- fmt.Errorf("subscribeNewHead failed to resubscribe")
				return
			}

		case newBlock := <-newBlockChan:
			block := Block{BlockNum: newBlock.Number.Uint64(), Timestamp: newBlock.Time}
			bcs.logger.Log(bcs.ctx, logging.LevelTrace, "detected new block", "block-num", block.BlockNum)
			bcs.updateEventTracker(&block, nil)
		}
	}
}

// updateEventTracker accepts a new block number and/or new event and dispatches a chain event if there are enough block confirmations
func (bcs *BaseChainService) updateEventTracker(block *Block, chainEvent *ethTypes.Log) {
	// lock the mutex for the shortest amount of time. The mutex only need to be locked to update the eventTracker data structure
	bcs.eventTracker.mu.Lock()

	if block != nil && block.BlockNum > bcs.eventTracker.latestBlock.BlockNum {
		bcs.eventTracker.latestBlock = *block
	}

	if chainEvent != nil {
		bcs.eventTracker.Push(*chainEvent)
		bcs.logger.Debug("event added to queue", "updated-queue-length", bcs.eventTracker.events.Len())
	}

	eventsToDispatch := []ethTypes.Log{}
	for bcs.eventTracker.events.Len() > 0 && bcs.eventTracker.latestBlock.BlockNum >= (bcs.eventTracker.events)[0].BlockNumber+REQUIRED_BLOCK_CONFIRMATIONS {
		chainEvent := bcs.eventTracker.Pop()
		bcs.logger.Debug("event popped from queue", "updated-queue-length", bcs.eventTracker.events.Len())

		// Ensure event & associated tx is still in the chain before adding to eventsToDispatch
		oldBlock, err := bcs.chain.BlockByNumber(context.Background(), new(big.Int).SetUint64(chainEvent.BlockNumber))
		if err != nil {
			bcs.logger.Error("failed to fetch block", "err", err)
			bcs.errorChan <- fmt.Errorf("failed to fetch block: %v", err)
			return
		}

		if oldBlock.Hash() != chainEvent.BlockHash {
			bcs.logger.Warn("dropping event because its block is no longer in the chain (possible re-org)", "blockNumber", chainEvent.BlockNumber, "blockHash", chainEvent.BlockHash)
			continue
		}

		eventsToDispatch = append(eventsToDispatch, chainEvent)
	}
	bcs.eventTracker.mu.Unlock()

	if bcs.DispatchChainEvents != nil {
		err := bcs.DispatchChainEvents(eventsToDispatch)
		if err != nil {
			bcs.errorChan <- fmt.Errorf("failed dispatchChainEvents: %w", err)
			return
		}
	}
}

func (bcs *BaseChainService) subscribeNewHead() (chan *ethTypes.Header, error) {
	newBlockChan := make(chan *ethTypes.Header)
	newBlockSub, err := bcs.chain.SubscribeNewHead(bcs.ctx, newBlockChan)
	if err != nil {
		return nil, fmt.Errorf("subscribeNewHead failed: %w", err)
	}
	bcs.newBlockSub = newBlockSub
	return newBlockChan, nil
}

func (bcs *BaseChainService) SubscribeForLogs(eventQuery ethereum.FilterQuery) (chan ethTypes.Log, error) {
	eventChan := make(chan ethTypes.Log)
	eventSub, err := bcs.chain.SubscribeFilterLogs(bcs.ctx, eventQuery, eventChan)
	if err != nil {
		return nil, fmt.Errorf("subscribeFilterLogs failed: %w", err)
	}
	bcs.eventSub = eventSub

	return eventChan, nil
}

// EventFeed returns the out chan, and narrows the type so that external consumers may only receive on it.
func (bcs *BaseChainService) EventFeed() <-chan Event {
	return bcs.out
}

func (bcs *BaseChainService) GetConsensusAppAddress() types.Address {
	return bcs.consensusAppAddress
}

func (bcs *BaseChainService) GetVirtualPaymentAppAddress() types.Address {
	return bcs.virtualPaymentAppAddress
}

func (bcs *BaseChainService) GetChainId() (*big.Int, error) {
	return bcs.chain.ChainID(bcs.ctx)
}

func (bcs *BaseChainService) GetLastConfirmedBlockNum() uint64 {
	var confirmedBlockNum uint64

	bcs.eventTracker.mu.Lock()
	defer bcs.eventTracker.mu.Unlock()

	// Check for potential underflow
	if bcs.eventTracker.latestBlock.BlockNum >= REQUIRED_BLOCK_CONFIRMATIONS {
		confirmedBlockNum = bcs.eventTracker.latestBlock.BlockNum - REQUIRED_BLOCK_CONFIRMATIONS
	} else {
		confirmedBlockNum = 0
	}

	return confirmedBlockNum
}

func (bcs *BaseChainService) GetLatestBlock() Block {
	bcs.eventTracker.mu.Lock()
	defer bcs.eventTracker.mu.Unlock()
	return bcs.eventTracker.latestBlock
}

func (bcs *BaseChainService) Close() error {
	bcs.cancel()
	bcs.Wg.Wait()
	return nil
}
