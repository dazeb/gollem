package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// RequestID is a JSON-RPC request id. Gollem preserves the original JSON
// representation so string and numeric ids round-trip without lossy casts.
type RequestID struct {
	raw json.RawMessage
}

// NewStringID creates a string request id.
func NewStringID(id string) RequestID {
	data, _ := json.Marshal(id)
	return RequestID{raw: data}
}

// NewNumberID creates a numeric request id.
func NewNumberID(id int64) RequestID {
	data, _ := json.Marshal(id)
	return RequestID{raw: data}
}

// IsZero reports whether the id is unset.
func (id RequestID) IsZero() bool {
	return len(bytes.TrimSpace(id.raw)) == 0
}

// Raw returns a copy of the encoded JSON id.
func (id RequestID) Raw() json.RawMessage {
	if id.IsZero() {
		return nil
	}
	return append(json.RawMessage(nil), id.raw...)
}

// Value decodes the id to string or json.Number.
func (id RequestID) Value() any {
	if id.IsZero() {
		return nil
	}
	var s string
	if err := json.Unmarshal(id.raw, &s); err == nil {
		return s
	}
	var n json.Number
	dec := json.NewDecoder(bytes.NewReader(id.raw))
	dec.UseNumber()
	if err := dec.Decode(&n); err == nil {
		return n
	}
	return nil
}

// MarshalJSON writes the original string or numeric JSON id.
func (id RequestID) MarshalJSON() ([]byte, error) {
	if id.IsZero() {
		return []byte("null"), nil
	}
	return append([]byte(nil), id.raw...), nil
}

// UnmarshalJSON accepts string and numeric request ids.
func (id *RequestID) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return errors.New("request id must be a string or number")
	}
	var s string
	if err := json.Unmarshal(trimmed, &s); err == nil {
		id.raw = append(id.raw[:0], trimmed...)
		return nil
	}
	var n json.Number
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	if err := dec.Decode(&n); err == nil {
		if _, err := n.Int64(); err == nil {
			id.raw = append(id.raw[:0], trimmed...)
			return nil
		}
		return fmt.Errorf("request id number must be an integer: %s", n)
	}
	return fmt.Errorf("request id must be a string or number: %s", string(trimmed))
}
