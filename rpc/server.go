package rpc

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/statechannels/go-nitro/rand"
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

func processRequest[T serde.RequestPayload, U serde.ResponsePayload](rs *BaseRpcServer, permission permission, requestData []byte, processPayload func(T) (U, error)) []byte {
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

func sendNotification[T serde.NotificationMethod, U serde.NotificationPayload](rs *BaseRpcServer, method T, payload U) error {
	rs.logger.Debug("Sending notification", "method", method, "payload", payload)

	request := serde.NewJsonRpcSpecificRequest(rand.Uint64(), method, payload, "")
	data, err := json.Marshal(request)
	if err != nil {
		return err
	}
	return rs.transport.Notify(data)
}

// Marshal and return response data
func marshalResponse(response any) []byte {
	responseData, err := json.Marshal(response)
	if err != nil {
		slog.Error("Could not marshal response", "error", err)
	}
	return responseData
}

func validateJsonrpcRequest(requestData []byte) (serde.JsonRpcGeneralRequest, []byte) {
	var request map[string]interface{}
	vr := serde.JsonRpcGeneralRequest{}
	err := json.Unmarshal(requestData, &request)
	if err != nil {
		errRes := serde.NewJsonRpcErrorResponse(0, serde.RequestUnmarshalError)
		return serde.JsonRpcGeneralRequest{}, marshalResponse(errRes)
	}

	// jsonrpc spec says id can be a string, number.
	// We only support numbers: https://github.com/statechannels/go-nitro/issues/1160
	// When golang unmarshals JSON into an interface value, float64 is used for numbers.
	requestId := request["id"]
	fRequestId, ok := requestId.(float64)
	if !ok || fRequestId != float64(uint64(fRequestId)) {
		errRes := serde.NewJsonRpcErrorResponse(0, serde.InvalidRequestError)
		return serde.JsonRpcGeneralRequest{}, marshalResponse(errRes)
	}
	vr.Id = uint64(fRequestId)

	sJsonrpc, ok := request["jsonrpc"].(string)
	if !ok || sJsonrpc != "2.0" {
		errRes := serde.NewJsonRpcErrorResponse(vr.Id, serde.InvalidRequestError)
		return serde.JsonRpcGeneralRequest{}, marshalResponse(errRes)
	}

	sMethod, ok := request["method"].(string)
	if !ok {
		errRes := serde.NewJsonRpcErrorResponse(vr.Id, serde.InvalidRequestError)
		return serde.JsonRpcGeneralRequest{}, marshalResponse(errRes)
	}
	vr.Method = sMethod

	vr.Params = request["params"]
	return vr, nil
}
