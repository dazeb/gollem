package protocol

import (
	"encoding/json"
	"errors"
)

// Request is a client-to-server app-server request.
type Request struct {
	JSONRPC string          `json:"-"`
	ID      RequestID       `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Notification is a JSON-RPC notification without a response id.
type Notification struct {
	JSONRPC string          `json:"-"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is a server response to a request.
type Response struct {
	JSONRPC string          `json:"-"`
	ID      RequestID       `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// MarshalJSON emits a Codex-style request envelope without a jsonrpc member.
func (r Request) MarshalJSON() ([]byte, error) {
	type wire struct {
		ID     RequestID       `json:"id"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params,omitempty"`
	}
	return json.Marshal(wire{ID: r.ID, Method: r.Method, Params: r.Params})
}

// UnmarshalJSON accepts requests with or without the jsonrpc member.
func (r *Request) UnmarshalJSON(data []byte) error {
	type wire struct {
		JSONRPC string          `json:"jsonrpc,omitempty"`
		ID      RequestID       `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params,omitempty"`
	}
	var w wire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	if w.ID.IsZero() {
		return errors.New("request id is required")
	}
	if w.Method == "" {
		return errors.New("request method is required")
	}
	*r = Request(w)
	return nil
}

// MarshalJSON emits a Codex-style notification without a jsonrpc member.
func (n Notification) MarshalJSON() ([]byte, error) {
	type wire struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params,omitempty"`
	}
	return json.Marshal(wire{Method: n.Method, Params: n.Params})
}

// UnmarshalJSON accepts notifications with or without the jsonrpc member.
func (n *Notification) UnmarshalJSON(data []byte) error {
	type wire struct {
		JSONRPC string          `json:"jsonrpc,omitempty"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params,omitempty"`
	}
	var w wire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	if w.Method == "" {
		return errors.New("notification method is required")
	}
	*n = Notification(w)
	return nil
}

// MarshalJSON emits a Codex-style response without a jsonrpc member.
func (r Response) MarshalJSON() ([]byte, error) {
	type wire struct {
		ID     RequestID       `json:"id"`
		Result json.RawMessage `json:"result,omitempty"`
		Error  *Error          `json:"error,omitempty"`
	}
	return json.Marshal(wire{ID: r.ID, Result: r.Result, Error: r.Error})
}

// UnmarshalJSON accepts responses with or without the jsonrpc member.
func (r *Response) UnmarshalJSON(data []byte) error {
	type wire struct {
		JSONRPC string          `json:"jsonrpc,omitempty"`
		ID      RequestID       `json:"id"`
		Result  json.RawMessage `json:"result,omitempty"`
		Error   *Error          `json:"error,omitempty"`
	}
	var w wire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	if w.ID.IsZero() {
		return errors.New("response id is required")
	}
	if len(w.Result) > 0 && w.Error != nil {
		return errors.New("response cannot contain both result and error")
	}
	*r = Response(w)
	return nil
}
