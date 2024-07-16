package rpc

import (
	"encoding/json"
	"log/slog"

	"github.com/statechannels/go-nitro/bridge"
	"github.com/statechannels/go-nitro/internal/logging"
	"github.com/statechannels/go-nitro/node/query"
	"github.com/statechannels/go-nitro/rpc/serde"
	"github.com/statechannels/go-nitro/rpc/transport"
)

type BridgeRpcServer struct {
	*BaseRpcServer
	bridge *bridge.Bridge
}

func NewBridgeRpcServer(bridge *bridge.Bridge, trans transport.Responder) (*BridgeRpcServer, error) {
	baseRpcServer := NewBaseRpcServer(trans)

	brs := &BridgeRpcServer{
		baseRpcServer,
		bridge,
	}

	brs.logger = logging.LoggerWithAddress(slog.Default(), bridge.GetBridgeAddress())

	err := brs.registerHandlers()
	if err != nil {
		return nil, err
	}

	return brs, nil
}

func (brs *BridgeRpcServer) Close() error {
	err := brs.BaseRpcServer.Close()
	if err != nil {
		return err
	}

	return brs.bridge.Close()
}

func (brs *BridgeRpcServer) registerHandlers() (err error) {
	handlerV1 := func(requestData []byte) []byte {
		if !json.Valid(requestData) {
			brs.logger.Error("request is not valid json")
			errRes := serde.NewJsonRpcErrorResponse(0, serde.ParseError)
			return marshalResponse(errRes)
		}

		jsonrpcReq, errRes := validateJsonrpcRequest(requestData)
		brs.logger.Debug("Rpc server received request", "request", jsonrpcReq)
		if errRes != nil {
			brs.logger.Error("could not validate jsonrpc request")

			return errRes
		}

		switch serde.RequestMethod(jsonrpcReq.Method) {
		case serde.GetAuthTokenMethod:
			return processRequest(brs.BaseRpcServer, permNone, requestData, func(req serde.AuthRequest) (string, error) {
				return generateAuthToken(req.Id, allPermissions)
			})
		case serde.GetAllL2ChannelsRequestMethod:
			return processRequest(brs.BaseRpcServer, permSign, requestData, func(req serde.NoPayloadRequest) ([]query.LedgerChannelInfo, error) {
				return brs.bridge.GetAllL2Channels()
			})
		default:
			errRes := serde.NewJsonRpcErrorResponse(jsonrpcReq.Id, serde.MethodNotFoundError)
			return marshalResponse(errRes)
		}
	}

	err = brs.transport.RegisterRequestHandler("v1", handlerV1)
	return err
}
