package rpc

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/statechannels/go-nitro/rpc/serde"
	"github.com/statechannels/go-nitro/rpc/transport"
)

type BaseRpcServer struct {
	transport transport.Responder
	logger    *slog.Logger
	cancel    context.CancelFunc
	wg        *sync.WaitGroup
}

func (rs *BaseRpcServer) Url() string {
	return rs.transport.Url()
}

func (rs *BaseRpcServer) Close() error {
	return rs.transport.Close()
}

func NewBaseRpcServer(trans transport.Responder) *BaseRpcServer {
	rs := &BaseRpcServer{
		transport: trans,
		wg:        &sync.WaitGroup{},
		logger:    slog.Default(),
	}

	return rs
}

// TODO: Make this function generic and implement nodeProcessRequest and bridgeProcessRequest by creating wrapper around generic baseProcessRequest
func baseProcessRequest[T serde.RequestPayload, U serde.ResponsePayload](rs *BaseRpcServer, permission permission, requestData []byte, processPayload func(T) (U, error)) []byte {
	rpcRequest := serde.JsonRpcSpecificRequest[T]{}
	// This unmarshal will fail only when the requestData is not valid json.
	// Request-specific params validation is optionally performed as part of the processPayload function
	err := json.Unmarshal(requestData, &rpcRequest)
	if err != nil {
		response := serde.NewJsonRpcErrorResponse(rpcRequest.Id, serde.ParamsUnmarshalError)
		return marshalResponse(response)
	}

	err = checkTokenValidity(rpcRequest.Params.AuthToken, permission, 7*24*time.Hour)
	if err != nil {
		response := serde.NewJsonRpcErrorResponse(rpcRequest.Id, serde.InvalidAuthTokenError)
		rs.logger.Warn(serde.InvalidAuthTokenError.Message)
		return marshalResponse(response)
	}

	payload := rpcRequest.Params.Payload
	processedResponse, err := processPayload(payload)
	if err != nil {
		responseErr := serde.InternalServerError // default error
		responseErr.Message = err.Error()

		if jsonErr, ok := err.(serde.JsonRpcError); ok {
			responseErr.Code = jsonErr.Code // overwrite default if error object is jsonrpc error
		}

		response := serde.NewJsonRpcErrorResponse(rpcRequest.Id, responseErr)
		return marshalResponse(response)
	}

	response := serde.NewJsonRpcResponse(rpcRequest.Id, processedResponse)
	return marshalResponse(response)
}
