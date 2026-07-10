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

// OverloadedData describes a retryable app-server backpressure rejection.
type OverloadedData struct {
	Method           string `json:"method,omitempty"`
	Limit            int    `json:"limit,omitempty"`
	Pending          int    `json:"pending,omitempty"`
	Retryable        bool   `json:"retryable"`
	RetryAfterMillis int    `json:"retryAfterMillis,omitempty"`
	Reason           string `json:"reason"`
}

// OverloadedError returns the Codex-compatible app-server backpressure error.
func OverloadedError(method string, limit, pending int, reason string) *Error {
	if reason == "" {
		reason = "app-server request queue is full"
	}
	data, _ := json.Marshal(OverloadedData{
		Method:           method,
		Limit:            limit,
		Pending:          pending,
		Retryable:        true,
		RetryAfterMillis: 100,
		Reason:           reason,
	})
	return &Error{
		Code:    CodeOverloaded,
		Message: "app-server overloaded",
		Data:    data,
	}
}

// MethodUnavailableError returns a typed unavailable error for a known method.
// Unknown methods still receive JSON-RPC method-not-found.
func MethodUnavailableError(method string) *Error {
	return MethodUnavailableErrorWithReason(method, "method is registered in the app-server contract but is not implemented in this Gollem build")
}

// MethodUnavailableErrorWithReason returns a typed unavailable error with a
// runtime-specific reason. Unknown methods still receive method-not-found.
func MethodUnavailableErrorWithReason(method, reason string) *Error {
	info, ok := LookupMethod(method)
	if !ok {
		return &Error{Code: CodeMethodNotFound, Message: "method not found"}
	}
	if reason == "" {
		reason = "method is unavailable"
	}
	data, _ := json.Marshal(UnavailableData{
		Method:  method,
		Surface: info.Surface,
		Status:  info.State,
		Reason:  reason,
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
