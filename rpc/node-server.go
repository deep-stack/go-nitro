package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"

	"github.com/statechannels/go-nitro/internal/logging"
	nitro "github.com/statechannels/go-nitro/node"
	"github.com/statechannels/go-nitro/node/query"
	"github.com/statechannels/go-nitro/payments"
	"github.com/statechannels/go-nitro/protocols"
	"github.com/statechannels/go-nitro/protocols/bridgeddefund"
	"github.com/statechannels/go-nitro/protocols/directdefund"
	"github.com/statechannels/go-nitro/protocols/directfund"
	"github.com/statechannels/go-nitro/protocols/virtualdefund"
	"github.com/statechannels/go-nitro/protocols/virtualfund"
	"github.com/statechannels/go-nitro/rpc/serde"
	"github.com/statechannels/go-nitro/rpc/transport"
	"github.com/statechannels/go-nitro/types"
)

const DISABLE_BRIDGE_DEFUND = true

type NodeRpcServer struct {
	*BaseRpcServer
	node *nitro.Node
}

// newNodeRpcServerWithoutNotifications creates a new rpc server without notifications enabled
func newNodeRpcServerWithoutNotifications(nitroNode *nitro.Node, trans transport.Responder) (*NodeRpcServer, error) {
	baseRpcServer := NewBaseRpcServer(trans)
	nrs := &NodeRpcServer{
		baseRpcServer,
		nitroNode,
	}

	if hasNitroAddress := (nitroNode.Address != nil) && (nitroNode.Address != &types.Address{}); hasNitroAddress {
		nrs.logger = logging.LoggerWithAddress(slog.Default(), *nitroNode.Address)
	}

	err := nrs.registerHandlers()
	if err != nil {
		return nil, err
	}

	return nrs, nil
}

func NewNodeRpcServer(node *nitro.Node, trans transport.Responder) (*NodeRpcServer, error) {
	baseRpcServer := NewBaseRpcServer(trans)
	nrs := &NodeRpcServer{
		baseRpcServer,
		node,
	}

	nrs.logger = logging.LoggerWithAddress(slog.Default(), *node.Address)
	ctx, cancel := context.WithCancel(context.Background())
	nrs.cancel = cancel
	nrs.wg.Add(1)

	// The update channels are initialized syncronously.
	// If these channels are initialized in another go routine,
	// the server can send an update before the channels are initialized.
	completedObjChan := nrs.node.CompletedObjectives()
	ledgerUpdateChan := nrs.node.LedgerUpdates()
	paymentUpdateChan := nrs.node.PaymentUpdates()

	go nrs.sendNotifications(ctx, completedObjChan, ledgerUpdateChan, paymentUpdateChan)

	err := nrs.registerHandlers()
	if err != nil {
		return nil, err
	}

	return nrs, nil
}

func (nrs *NodeRpcServer) Close() error {
	err := nrs.BaseRpcServer.Close()
	if err != nil {
		return err
	}

	return nrs.node.Close()
}

// registerHandlers registers the handlers for the rpc server
func (nrs *NodeRpcServer) registerHandlers() (err error) {
	handlerV1 := func(requestData []byte) []byte {
		if !json.Valid(requestData) {
			nrs.logger.Error("request is not valid json")
			errRes := serde.NewJsonRpcErrorResponse(0, serde.ParseError)
			return marshalResponse(errRes)
		}

		jsonrpcReq, errRes := validateJsonrpcRequest(requestData)
		nrs.logger.Debug("Rpc server received request", "request", jsonrpcReq)
		if errRes != nil {
			nrs.logger.Error("could not validate jsonrpc request")

			return errRes
		}

		switch serde.RequestMethod(jsonrpcReq.Method) {
		case serde.GetAuthTokenMethod:
			return processRequest(nrs.BaseRpcServer, permNone, requestData, func(req serde.AuthRequest) (string, error) {
				return generateAuthToken(req.Id, allPermissions)
			})
		case serde.CreateVoucherRequestMethod:
			return processRequest(nrs.BaseRpcServer, permSign, requestData, func(req serde.PaymentRequest) (payments.Voucher, error) {
				return nrs.node.CreateVoucher(req.Channel, big.NewInt(int64(req.Amount)))
			})
		case serde.ReceiveVoucherRequestMethod:
			return processRequest(nrs.BaseRpcServer, permRead, requestData, func(req payments.Voucher) (payments.ReceiveVoucherSummary, error) {
				return nrs.node.ReceiveVoucher(req)
			})
		case serde.GetAddressMethod:
			return processRequest(nrs.BaseRpcServer, permNone, requestData, func(req serde.NoPayloadRequest) (string, error) {
				return nrs.node.Address.Hex(), nil
			})
		case serde.VersionMethod:
			return processRequest(nrs.BaseRpcServer, permNone, requestData, func(req serde.NoPayloadRequest) (string, error) {
				return nrs.node.Version(), nil
			})
		case serde.CreateLedgerChannelRequestMethod:
			return processRequest(nrs.BaseRpcServer, permSign, requestData, func(req directfund.ObjectiveRequest) (directfund.ObjectiveResponse, error) {
				return nrs.node.CreateLedgerChannel(req.CounterParty, req.ChallengeDuration, req.Outcome)
			})
		case serde.CloseLedgerChannelRequestMethod:
			return processRequest(nrs.BaseRpcServer, permSign, requestData, func(req directdefund.ObjectiveRequest) (protocols.ObjectiveId, error) {
				return nrs.node.CloseLedgerChannel(req.ChannelId, req.IsChallenge)
			})
		case serde.CloseBridgeChannelRequestMethod:
			return processRequest(nrs.BaseRpcServer, permSign, requestData, func(req bridgeddefund.ObjectiveRequest) (protocols.ObjectiveId, error) {
				if DISABLE_BRIDGE_DEFUND {
					return protocols.ObjectiveId(bridgeddefund.ObjectivePrefix + req.ChannelId.String()), fmt.Errorf("brided defund is currently disabled")
				}
				return nrs.node.CloseBridgeChannel(req.ChannelId)
			})
		case serde.CreatePaymentChannelRequestMethod:
			return processRequest(nrs.BaseRpcServer, permSign, requestData, func(req virtualfund.ObjectiveRequest) (virtualfund.ObjectiveResponse, error) {
				return nrs.node.CreatePaymentChannel(req.Intermediaries, req.CounterParty, req.ChallengeDuration, req.Outcome)
			})
		case serde.ClosePaymentChannelRequestMethod:
			return processRequest(nrs.BaseRpcServer, permSign, requestData, func(req virtualdefund.ObjectiveRequest) (protocols.ObjectiveId, error) {
				return nrs.node.ClosePaymentChannel(req.ChannelId)
			})
		case serde.PayRequestMethod:
			return processRequest(nrs.BaseRpcServer, permSign, requestData, func(req serde.PaymentRequest) (serde.PaymentRequest, error) {
				if err := serde.ValidatePaymentRequest(req); err != nil {
					return serde.PaymentRequest{}, err
				}
				nrs.node.Pay(req.Channel, big.NewInt(int64(req.Amount)))
				return req, nil
			})
		case serde.GetPaymentChannelRequestMethod:
			return processRequest(nrs.BaseRpcServer, permRead, requestData, func(req serde.GetPaymentChannelRequest) (query.PaymentChannelInfo, error) {
				if err := serde.ValidateGetPaymentChannelRequest(req); err != nil {
					return query.PaymentChannelInfo{}, err
				}
				return nrs.node.GetPaymentChannel(req.Id)
			})
		case serde.GetLedgerChannelRequestMethod:
			return processRequest(nrs.BaseRpcServer, permRead, requestData, func(req serde.GetLedgerChannelRequest) (query.LedgerChannelInfo, error) {
				return nrs.node.GetLedgerChannel(req.Id)
			})
		case serde.GetAllLedgerChannelsMethod:
			return processRequest(nrs.BaseRpcServer, permRead, requestData, func(req serde.NoPayloadRequest) ([]query.LedgerChannelInfo, error) {
				return nrs.node.GetAllLedgerChannels()
			})
		case serde.GetPaymentChannelsByLedgerMethod:
			return processRequest(nrs.BaseRpcServer, permRead, requestData, func(req serde.GetPaymentChannelsByLedgerRequest) ([]query.PaymentChannelInfo, error) {
				if err := serde.ValidateGetPaymentChannelsByLedgerRequest(req); err != nil {
					return []query.PaymentChannelInfo{}, err
				}
				return nrs.node.GetPaymentChannelsByLedger(req.LedgerId)
			})
		case serde.CounterChallengeRequestMethod:
			return processRequest(nrs.BaseRpcServer, permSign, requestData, func(req serde.CounterChallengeRequest) (serde.CounterChallengeRequest, error) {
				// TODO: Unmarshall the signed state string
				nrs.node.CounterChallenge(req.ChannelId, req.Action, req.Payload)
				return req, nil
			})
		default:
			errRes := serde.NewJsonRpcErrorResponse(jsonrpcReq.Id, serde.MethodNotFoundError)
			return marshalResponse(errRes)
		}
	}

	err = nrs.transport.RegisterRequestHandler("v1", handlerV1)
	return err
}

func (rs *NodeRpcServer) sendNotifications(ctx context.Context,
	completedObjChan <-chan protocols.ObjectiveId,
	ledgerUpdatesChan <-chan query.LedgerChannelInfo,
	paymentUpdatesChan <-chan query.PaymentChannelInfo,
) {
	defer rs.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return

		case completedObjective, ok := <-completedObjChan:
			if !ok {
				rs.logger.Warn("CompletedObjectives channel closed, exiting sendNotifications")
				return
			}
			err := sendNotification(rs.BaseRpcServer, serde.ObjectiveCompleted, completedObjective)
			if err != nil {
				panic(err)
			}
		case ledgerInfo, ok := <-ledgerUpdatesChan:
			if !ok {
				rs.logger.Warn("LedgerUpdates channel closed, exiting sendNotifications")
				return
			}
			err := sendNotification(rs.BaseRpcServer, serde.LedgerChannelUpdated, ledgerInfo)
			if err != nil {
				panic(err)
			}
		case paymentInfo, ok := <-paymentUpdatesChan:
			if !ok {
				rs.logger.Warn("PaymentUpdates channel closed, exiting sendNotifications")
				return
			}
			err := sendNotification(rs.BaseRpcServer, serde.PaymentChannelUpdated, paymentInfo)
			if err != nil {
				panic(err)
			}
		}
	}
}
