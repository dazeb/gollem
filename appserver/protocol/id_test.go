package protocol

import (
	"encoding/json"
	"testing"
)

func TestRequestIDRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		id   RequestID
		want string
	}{
		{"string", NewStringID("req-1"), `"req-1"`},
		{"number", NewNumberID(42), `42`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.id)
			if err != nil {
				t.Fatalf("MarshalJSON: %v", err)
			}
			if string(data) != tt.want {
				t.Fatalf("marshal = %s, want %s", data, tt.want)
			}
			var decoded RequestID
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("UnmarshalJSON: %v", err)
			}
			again, err := json.Marshal(decoded)
			if err != nil {
				t.Fatalf("remarshal: %v", err)
			}
			if string(again) != tt.want {
				t.Fatalf("round-trip = %s, want %s", again, tt.want)
			}
		})
	}
}

func TestRequestIDRejectsInvalidJSONRPCIDs(t *testing.T) {
	for _, input := range []string{`null`, `true`, `{}`, `[]`, `1.25`} {
		t.Run(input, func(t *testing.T) {
			var id RequestID
			if err := json.Unmarshal([]byte(input), &id); err == nil {
				t.Fatalf("expected %s to be rejected", input)
			}
		})
	}
}
