package chainservice

import (
	"context"
	"fmt"
	"log/slog"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/statechannels/go-nitro/channel/state"
	NitroAdjudicator "github.com/statechannels/go-nitro/node/engine/chainservice/adjudicator"
	Token "github.com/statechannels/go-nitro/node/engine/chainservice/erc20"
	chainutils "github.com/statechannels/go-nitro/node/engine/chainservice/utils"
	"github.com/statechannels/go-nitro/protocols"
)

var l1topicsToWatch = []common.Hash{
	allocationUpdatedTopic,
	concludedTopic,
	depositedTopic,
	challengeRegisteredTopic,
	challengeClearedTopic,
	reclaimedTopic,
}

var (
	naAbi, _                 = NitroAdjudicator.NitroAdjudicatorMetaData.GetAbi()
	concludedTopic           = naAbi.Events["Concluded"].ID
	allocationUpdatedTopic   = naAbi.Events["AllocationUpdated"].ID
	depositedTopic           = naAbi.Events["Deposited"].ID
	challengeRegisteredTopic = naAbi.Events["ChallengeRegistered"].ID
	challengeClearedTopic    = naAbi.Events["ChallengeCleared"].ID
	reclaimedTopic           = naAbi.Events["Reclaimed"].ID
)

type EthChainService struct {
	*BaseChainService
	na        *NitroAdjudicator.NitroAdjudicator
	naAddress common.Address
}

// NewEthChainService is a convenient wrapper around newEthChainService, which provides a simpler API
func NewEthChainService(chainOpts ChainOpts) (ChainService, error) {
	if chainOpts.ChainPk == "" {
		return nil, fmt.Errorf("chainpk must be set")
	}
	if chainOpts.VpaAddress == chainOpts.CaAddress {
		return nil, fmt.Errorf("virtual payment app address and consensus app address cannot be the same: %s", chainOpts.VpaAddress.String())
	}

	ethClient, txSigner, err := chainutils.ConnectToChain(
		context.Background(),
		chainOpts.ChainUrl,
		chainOpts.ChainAuthToken,
		common.Hex2Bytes(chainOpts.ChainPk),
	)
	if err != nil {
		panic(err)
	}

	na, err := NitroAdjudicator.NewNitroAdjudicator(chainOpts.NaAddress, ethClient)
	if err != nil {
		panic(err)
	}

	return newEthChainService(ethClient, chainOpts.ChainStartBlockNum, na, chainOpts.NaAddress, chainOpts.CaAddress, chainOpts.VpaAddress, txSigner)
}

func newEthChainService(chain ethChain, startBlockNum uint64, na *NitroAdjudicator.NitroAdjudicator,
	naAddress, caAddress, vpaAddress common.Address, txSigner *bind.TransactOpts,
) (*EthChainService, error) {
	baseCS, err := NewBaseChainService(chain, startBlockNum, txSigner, caAddress, vpaAddress)
	if err != nil {
		panic(err)
	}

	ecs := EthChainService{
		BaseChainService: baseCS,
		na:               na,
		naAddress:        naAddress,
	}
	baseCS.DispatchChainEvents = ecs.DispatchChainEvents

	eventQuery := ethereum.FilterQuery{
		Addresses: []common.Address{ecs.naAddress},
		Topics:    [][]common.Hash{l1topicsToWatch},
	}

	eventChan, err := ecs.SubscribeForLogs(eventQuery)
	if err != nil {
		return &EthChainService{}, nil
	}

	ecs.Wg.Add(1)
	go ecs.ListenForEventLogs(eventChan, eventQuery)

	// Search for any missed events emitted while this node was offline
	err = ecs.CheckForMissedEvents(startBlockNum, eventQuery)
	if err != nil {
		return nil, err
	}

	return &ecs, nil
}

// SendTransaction sends the transaction and blocks until it has been submitted.
func (ecs *EthChainService) SendTransaction(tx protocols.ChainTransaction) error {
	switch tx := tx.(type) {
	case protocols.DepositTransaction:
		for tokenAddress, amount := range tx.Deposit {
			txOpts := ecs.defaultTxOpts()
			ethTokenAddress := common.Address{}
			if tokenAddress == ethTokenAddress {
				txOpts.Value = amount
			} else {
				token, err := Token.NewToken(tokenAddress, ecs.chain)
				if err != nil {
					return err
				}

				approvalLogsChan := make(chan *Token.TokenApproval)

				approvalSubscription, err := token.WatchApproval(&bind.WatchOpts{Context: ecs.ctx}, approvalLogsChan, []common.Address{ecs.txSigner.From}, []common.Address{ecs.naAddress})
				if err != nil {
					return err
				}

				approveTx, err := token.Approve(ecs.defaultTxOpts(), ecs.naAddress, amount)
				if err != nil {
					return err
				}

				// Get current block
				currentBlock := <-ecs.newBlockChan

				isApproveTxRetried := false

				// Transaction hash of retried Approve transaction
				var retryApproveTxHash common.Hash

				// Wait for the Approve transaction to be mined before continuing
			approvalEventListenerLoop:
				for {
					select {
					case log := <-approvalLogsChan:
						if log.Owner == ecs.txSigner.From {
							approvalSubscription.Unsubscribe()
							break approvalEventListenerLoop
						}
					case err := <-approvalSubscription.Err():
						return err
					case newBlock := <-ecs.newBlockChan:
						if (newBlock.Number.Int64() - currentBlock.Number.Int64()) > BLOCKS_WITHOUT_EVENT_THRESHOLD {
							if isApproveTxRetried {
								slog.Error("approve transaction was retried with higher gas and event Approval was not emitted till latest block", "txHash", retryApproveTxHash, "latestBlock", newBlock.Number.String())
								return nil
							}

							slog.Error("event Approval was not emitted", "approveTxHash", approveTx.Hash().String())

							// Estimate gas for new Approve transaction
							estimatedGasLimit, err := ecs.estimateGasForApproveTx(tokenAddress, amount)
							if err != nil {
								return err
							}

							approveTxOpts := ecs.defaultTxOpts()

							// Multiply estimated gas limit with set multiplier
							approveTxOpts.GasLimit = uint64(float64(estimatedGasLimit) * GAS_LIMIT_MULTIPLIER)
							reApproveTx, err := token.Approve(approveTxOpts, ecs.naAddress, amount)
							if err != nil {
								return err
							}

							isApproveTxRetried = true
							currentBlock = newBlock
							retryApproveTxHash = reApproveTx.Hash()

							slog.Info("Resubmitted transaction with higher gas limit", "gasLimit", approveTxOpts.GasLimit, "approveTxHash", reApproveTx.Hash().String())
						}
					}
				}
			}

			holdings, err := ecs.na.Holdings(&bind.CallOpts{}, tokenAddress, tx.ChannelId())
			ecs.logger.Debug("existing holdings", "holdings", holdings)

			if err != nil {
				return err
			}

			_, err = ecs.na.Deposit(txOpts, tokenAddress, tx.ChannelId(), holdings, amount)
			if err != nil {
				return err
			}
		}
		return nil
	case protocols.WithdrawAllTransaction:
		signedState := tx.SignedState.State()
		signatures := tx.SignedState.Signatures()
		nitroFixedPart := NitroAdjudicator.INitroTypesFixedPart(NitroAdjudicator.ConvertFixedPart(signedState.FixedPart()))
		nitroVariablePart := NitroAdjudicator.ConvertVariablePart(signedState.VariablePart())
		nitroSignatures := []NitroAdjudicator.INitroTypesSignature{NitroAdjudicator.ConvertSignature(signatures[0]), NitroAdjudicator.ConvertSignature(signatures[1])}

		candidate := NitroAdjudicator.INitroTypesSignedVariablePart{
			VariablePart: nitroVariablePart,
			Sigs:         nitroSignatures,
		}
		_, err := ecs.na.ConcludeAndTransferAllAssets(ecs.defaultTxOpts(), nitroFixedPart, candidate)
		return err
	case protocols.ChallengeTransaction:
		fp, candidate := NitroAdjudicator.ConvertSignedStateToFixedPartAndSignedVariablePart(tx.Candidate)
		proof := NitroAdjudicator.ConvertSignedStatesToProof(tx.Proof)
		challengerSig := NitroAdjudicator.ConvertSignature(tx.ChallengerSig)
		_, err := ecs.na.Challenge(ecs.defaultTxOpts(), fp, proof, candidate, challengerSig)
		return err
	case protocols.CheckpointTransaction:
		fp, candidate := NitroAdjudicator.ConvertSignedStateToFixedPartAndSignedVariablePart(tx.Candidate)
		proof := NitroAdjudicator.ConvertSignedStatesToProof(tx.Proof)
		_, err := ecs.na.Checkpoint(ecs.defaultTxOpts(), fp, proof, candidate)
		return err
	case protocols.TransferAllTransaction:
		transferState := tx.TransferState.State()
		channelId := transferState.ChannelId()
		stateHash, err := transferState.Hash()
		if err != nil {
			return err
		}

		nitroVariablePart := NitroAdjudicator.ConvertVariablePart(transferState.VariablePart())

		_, er := ecs.na.TransferAllAssets(ecs.defaultTxOpts(), channelId, nitroVariablePart.Outcome, stateHash)
		return er
	case protocols.ReclaimTransaction:
		_, err := ecs.na.Reclaim(ecs.defaultTxOpts(), tx.ReclaimArgs)
		return err
	case protocols.MirrorTransferAllTransaction:
		transferState := tx.TransferState.State()
		channelId := transferState.ChannelId()
		stateHash, err := transferState.Hash()
		if err != nil {
			return err
		}

		nitroVariablePart := NitroAdjudicator.ConvertVariablePart(transferState.VariablePart())

		_, er := ecs.na.MirrorTransferAllAssets(ecs.defaultTxOpts(), channelId, nitroVariablePart.Outcome, stateHash)
		return er
	case protocols.SetL2ToL1Transaction:
		_, err := ecs.na.SetL2ToL1(ecs.defaultTxOpts(), tx.ChannelId(), tx.MirrorChannelId)
		return err
	case protocols.MirrorWithdrawAllTransaction:
		signedState := tx.SignedState.State()
		signatures := tx.SignedState.Signatures()
		nitroFixedPart := NitroAdjudicator.INitroTypesFixedPart(NitroAdjudicator.ConvertFixedPart(signedState.FixedPart()))
		nitroVariablePart := NitroAdjudicator.ConvertVariablePart(signedState.VariablePart())
		nitroSignatures := []NitroAdjudicator.INitroTypesSignature{NitroAdjudicator.ConvertSignature(signatures[0]), NitroAdjudicator.ConvertSignature(signatures[1])}

		candidate := NitroAdjudicator.INitroTypesSignedVariablePart{
			VariablePart: nitroVariablePart,
			Sigs:         nitroSignatures,
		}
		_, err := ecs.na.MirrorConcludeAndTransferAllAssets(ecs.defaultTxOpts(), nitroFixedPart, candidate)
		return err
	default:
		return fmt.Errorf("unexpected transaction type %T", tx)
	}
}

// dispatchChainEvents takes in a collection of event logs from the chain
// and dispatches events to the out channel
func (ecs *EthChainService) DispatchChainEvents(logs []ethTypes.Log) error {
	for _, l := range logs {
		block, err := ecs.chain.BlockByHash(context.Background(), l.BlockHash)
		if err != nil {
			return fmt.Errorf("error in getting block by hash %w", err)
		}

		switch l.Topics[0] {
		case depositedTopic:
			ecs.logger.Debug("Processing Deposited event")
			nad, err := ecs.na.ParseDeposited(l)
			if err != nil {
				return fmt.Errorf("error in ParseDeposited: %w", err)
			}

			event := NewDepositedEvent(nad.Destination, Block{BlockNum: l.BlockNumber, Timestamp: block.Time()}, l.TxIndex, nad.Asset, nad.DestinationHoldings)
			ecs.out <- event

		case allocationUpdatedTopic:
			ecs.logger.Debug("Processing AllocationUpdated event")
			au, err := ecs.na.ParseAllocationUpdated(l)
			if err != nil {
				return fmt.Errorf("error in ParseAllocationUpdated: %w", err)
			}

			tx, pending, err := ecs.chain.TransactionByHash(ecs.ctx, l.TxHash)
			if pending {
				return fmt.Errorf("expected transaction to be part of the chain, but the transaction is pending")
			}
			if err != nil {
				return fmt.Errorf("error in TransactionByHash: %w", err)
			}

			assetAddress, err := assetAddressForIndex(ecs.na, tx, au.AssetIndex)
			if err != nil {
				return fmt.Errorf("error in assetAddressForIndex: %w", err)
			}
			ecs.logger.Debug("assetAddress", "assetAddress", assetAddress)

			event := NewAllocationUpdatedEvent(au.ChannelId, Block{BlockNum: l.BlockNumber, Timestamp: block.Time()}, l.TxIndex, assetAddress, au.FinalHoldings)
			ecs.out <- event

		case concludedTopic:
			ecs.logger.Debug("Processing Concluded event")
			ce, err := ecs.na.ParseConcluded(l)
			if err != nil {
				return fmt.Errorf("error in ParseConcluded: %w", err)
			}

			event := ConcludedEvent{commonEvent: commonEvent{channelID: ce.ChannelId, block: Block{BlockNum: l.BlockNumber, Timestamp: block.Time()}}}
			ecs.out <- event

		case challengeRegisteredTopic:
			ecs.logger.Debug("Processing Challenge Registered event")

			tx, pending, err := ecs.chain.TransactionByHash(context.Background(), l.TxHash)
			if pending {
				return fmt.Errorf("expected transaction to be part of the chain, but the transaction is pending")
			}
			if err != nil {
				return fmt.Errorf("error in TransactionByHash: %w", err)
			}

			txSenderAddress, err := ecs.chain.TransactionSender(context.Background(), tx, l.BlockHash, l.TxIndex)
			if err != nil {
				return fmt.Errorf("error in TransactionSender: %w", err)
			}

			cr, err := ecs.na.ParseChallengeRegistered(l)
			if err != nil {
				return fmt.Errorf("error in ParseChallengeRegistered: %w", err)
			}
			isInitiatedByMe := txSenderAddress == ecs.txSigner.From

			event := NewChallengeRegisteredEvent(
				cr.ChannelId,
				Block{BlockNum: l.BlockNumber, Timestamp: block.Time()},
				l.TxIndex,
				state.VariablePart{
					AppData: cr.Candidate.VariablePart.AppData,
					Outcome: NitroAdjudicator.ConvertBindingsExitToExit(cr.Candidate.VariablePart.Outcome),
					TurnNum: cr.Candidate.VariablePart.TurnNum.Uint64(),
					IsFinal: cr.Candidate.VariablePart.IsFinal,
				},
				NitroAdjudicator.ConvertBindingsSignaturesToSignatures(cr.Candidate.Sigs),
				cr.FinalizesAt,
				isInitiatedByMe)
			ecs.out <- event
		case challengeClearedTopic:
			ecs.logger.Debug("Processing Challenge Cleared event")
			cp, err := ecs.na.ParseChallengeCleared(l)
			if err != nil {
				return fmt.Errorf("error in ParseCheckpointed: %w", err)
			}
			event := NewChallengeClearedEvent(cp.ChannelId, Block{BlockNum: l.BlockNumber, Timestamp: block.Time()}, l.TxIndex, cp.NewTurnNumRecord)
			ecs.out <- event

		case reclaimedTopic:
			ecs.logger.Debug("Processing Reclaimed event")
			ce, err := ecs.na.ParseReclaimed(l)
			if err != nil {
				return fmt.Errorf("error in ParseReclaimed: %w", err)
			}

			event := ReclaimedEvent{commonEvent: commonEvent{channelID: ce.ChannelId, block: Block{BlockNum: l.BlockNumber, Timestamp: block.Time()}, txIndex: l.TxIndex}}
			ecs.out <- event

		default:
			ecs.logger.Info("Ignoring unknown chain event topic", "topic", l.Topics[0].String())

		}
	}
	return nil
}

func (ecs *EthChainService) estimateGasForApproveTx(tokenAddress common.Address, amount *big.Int) (uint64, error) {
	parsedABI, err := abi.JSON(strings.NewReader(Token.TokenABI))
	if err != nil {
		return 0, fmt.Errorf("failed to parse ABI: %w", err)
	}

	data, err := parsedABI.Pack("approve", ecs.naAddress, amount)
	if err != nil {
		return 0, fmt.Errorf("failed to encode function call: %w", err)
	}

	callMsg := ethereum.CallMsg{
		From:     ecs.txSigner.From,
		To:       &tokenAddress,
		GasPrice: nil,
		Gas:      0,
		Value:    big.NewInt(0),
		Data:     data,
	}

	estimatedGasLimit, err := ecs.chain.EstimateGas(context.Background(), callMsg)
	if err != nil {
		return 0, fmt.Errorf("failed to estimate gas: %w", err)
	}

	return estimatedGasLimit, nil
}
