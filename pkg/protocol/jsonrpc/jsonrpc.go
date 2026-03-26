package jsonrpc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

const Version = "2.0"

var ErrUnknownMessage = errors.New("unknown JSON-RPC message shape")

type Request struct {
	JSONRPC string         `json:"jsonrpc"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params,omitempty"`
	ID      any            `json:"id,omitempty"`
}

type Response struct {
	JSONRPC string    `json:"jsonrpc"`
	Result  any       `json:"result,omitempty"`
	Error   *RPCError `json:"error,omitempty"`
	ID      any       `json:"id"`
}

type Event struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func NewRequest(method string, params map[string]any, id any) Request {
	if params == nil {
		params = map[string]any{}
	}
	return Request{
		JSONRPC: Version,
		Method:  method,
		Params:  params,
		ID:      id,
	}
}

func NewResponse(id any, result any) Response {
	return Response{JSONRPC: Version, Result: result, ID: id}
}

func NewErrorResponse(id any, code int, message string, data any) Response {
	return Response{
		JSONRPC: Version,
		Error:   &RPCError{Code: code, Message: message, Data: data},
		ID:      id,
	}
}

func NewEvent(method string, params any) Event {
	return Event{JSONRPC: Version, Method: method, Params: params}
}

func Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func DecodeMessage(data []byte) (any, error) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, err
	}

	if _, ok := probe["method"]; ok {
		if _, hasID := probe["id"]; hasID {
			var req Request
			if err := json.Unmarshal(data, &req); err != nil {
				return nil, err
			}
			return req, nil
		}

		var evt Event
		if err := json.Unmarshal(data, &evt); err != nil {
			return nil, err
		}
		return evt, nil
	}

	if _, ok := probe["result"]; ok || probe["error"] != nil {
		var resp Response
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, err
		}
		return resp, nil
	}

	return nil, ErrUnknownMessage
}

func MustVersion(raw string) error {
	if raw != Version {
		return fmt.Errorf("unexpected JSON-RPC version %q", raw)
	}
	return nil
}

func Compact(data []byte) string {
	var out bytes.Buffer
	if err := json.Compact(&out, data); err != nil {
		return string(data)
	}
	return out.String()
}
