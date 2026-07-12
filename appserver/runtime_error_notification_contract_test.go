package appserver

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"github.com/fugue-labs/gollem/appserver/protocol"
	"github.com/fugue-labs/gollem/appserver/store"
)

type runtimeErrorCaptureNotifier struct {
	method string
	params any
}

func (n *runtimeErrorCaptureNotifier) PublishNotification(method string, params any) {
	n.method = method
	n.params = params
}

func TestPublishRuntimeErrorRetainsLiveExtensionShape(t *testing.T) {
	for _, tc := range []struct {
		name     string
		turn     *store.Turn
		wantKeys []string
	}{
		{"thread turn", &store.Turn{ID: "turn", ThreadID: "thread"}, []string{"at", "error", "threadId", "turnId"}},
		{"global", nil, []string{"at", "error"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			notifier := &runtimeErrorCaptureNotifier{}
			publishRuntimeError(notifier, tc.turn, "boom")
			if notifier.method != "error" {
				t.Fatalf("method = %q", notifier.method)
			}
			params, ok := notifier.params.(runtimeErrorNotificationParams)
			if !ok || params.Error != "boom" || params.At.IsZero() {
				t.Fatalf("params = %#v (%T)", notifier.params, notifier.params)
			}
			if tc.turn != nil && (params.ThreadID != "thread" || params.TurnID != "turn") {
				t.Fatalf("correlated ids = %q/%q", params.ThreadID, params.TurnID)
			}
			if tc.turn == nil && (params.ThreadID != "" || params.TurnID != "") {
				t.Fatalf("global ids = %q/%q", params.ThreadID, params.TurnID)
			}
			encoded, err := json.Marshal(params)
			if err != nil {
				t.Fatal(err)
			}
			var payload map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &payload); err != nil {
				t.Fatal(err)
			}
			keys := make([]string, 0, len(payload))
			for key := range payload {
				keys = append(keys, key)
			}
			slices.Sort(keys)
			if !reflect.DeepEqual(keys, tc.wantKeys) {
				t.Fatalf("keys = %v, want %v", keys, tc.wantKeys)
			}
			var exact protocol.ErrorNotification
			if err := json.Unmarshal(encoded, &exact); err == nil {
				t.Fatal("incompatible live extension decoded as exact public error")
			}
		})
	}
}

func TestPublishRuntimeErrorNoopGuards(t *testing.T) {
	publishRuntimeError(nil, &store.Turn{ID: "turn", ThreadID: "thread"}, "boom")
	notifier := &runtimeErrorCaptureNotifier{}
	publishRuntimeError(notifier, &store.Turn{ID: "turn", ThreadID: "thread"}, "")
	if notifier.method != "" || notifier.params != nil {
		t.Fatalf("unexpected notification %q %#v", notifier.method, notifier.params)
	}
}
