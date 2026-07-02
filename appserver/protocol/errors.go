package protocol

import (
	"encoding/json"
	"fmt"
)

const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603

	// CodeOverloaded is the Codex-compatible app-server backpressure code.
	CodeOverloaded = -32001
	// CodeMethodUnavailable is used for known Codex-equivalent or Gollem
	// extension methods that are schema-defined but not implemented yet.
	CodeMethodUnavailable = -32004
)

// Error is a JSON-RPC error object.
type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("JSON-RPC error %d: %s", e.Code, e.Message)
}

// UnavailableData describes a known method that is not available yet.
type UnavailableData struct {
	Method  string      `json:"method"`
	Surface Surface     `json:"surface,omitempty"`
	Status  MethodState `json:"status,omitempty"`
	Reason  string      `json:"reason"`
}

// MethodUnavailableError returns a typed unavailable error for a known method.
// Unknown methods still receive JSON-RPC method-not-found.
func MethodUnavailableError(method string) *Error {
	info, ok := LookupMethod(method)
	if !ok {
		return &Error{Code: CodeMethodNotFound, Message: "method not found"}
	}
	data, _ := json.Marshal(UnavailableData{
		Method:  method,
		Surface: info.Surface,
		Status:  info.State,
		Reason:  "method is registered in the app-server contract but is not implemented in this Gollem build",
	})
	return &Error{
		Code:    CodeMethodUnavailable,
		Message: "method unavailable",
		Data:    data,
	}
}

// UnavailableResponse builds a JSON-RPC response for a known unavailable method.
func UnavailableResponse(id RequestID, method string) Response {
	return Response{ID: id, Error: MethodUnavailableError(method)}
}
