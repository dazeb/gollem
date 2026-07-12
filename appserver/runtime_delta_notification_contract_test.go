package appserver

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"github.com/fugue-labs/gollem/appserver/protocol"
	"github.com/fugue-labs/gollem/appserver/store"
	"github.com/fugue-labs/gollem/core"
)

type runtimeDeltaCaptureNotifier struct {
	method string
	params any
}

func (n *runtimeDeltaCaptureNotifier) PublishNotification(method string, params any) {
	n.method = method
	n.params = params
}

func TestPublishModelDeltaRetainsLiveExtensionShape(t *testing.T) {
	cases := []struct {
		kind       string
		wantMethod string
		exact      func([]byte) error
	}{
		{
			kind: "text", wantMethod: "item/agentMessage/delta",
			exact: func(data []byte) error {
				var value protocol.AgentMessageDeltaNotification
				return json.Unmarshal(data, &value)
			},
		},
		{
			kind: "thinking", wantMethod: "item/reasoning/textDelta",
			exact: func(data []byte) error {
				var value protocol.ReasoningTextDeltaNotification
				return json.Unmarshal(data, &value)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			notifier := &runtimeDeltaCaptureNotifier{}
			publishModelDelta(notifier, &store.Turn{ID: "turn", ThreadID: "thread"}, core.ModelDeltaEvent{
				PartIndex: 3, DeltaKind: tc.kind, ContentDelta: "delta",
			})
			if notifier.method != tc.wantMethod {
				t.Fatalf("method = %q, want %q", notifier.method, tc.wantMethod)
			}
			params, ok := notifier.params.(runtimeDeltaNotificationParams)
			if !ok {
				t.Fatalf("params = %T", notifier.params)
			}
			if params.ThreadID != "thread" || params.TurnID != "turn" || params.Delta != "delta" || params.Index != 3 || params.At.IsZero() {
				t.Fatalf("params = %#v", params)
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
			if want := []string{"at", "delta", "index", "threadId", "turnId"}; !reflect.DeepEqual(keys, want) {
				t.Fatalf("keys = %v, want %v", keys, want)
			}
			if err := tc.exact(encoded); err == nil {
				t.Fatal("incompatible live extension decoded as exact public notification")
			}
		})
	}
}

func TestPublishModelDeltaNoopGuards(t *testing.T) {
	event := core.ModelDeltaEvent{ContentDelta: "delta"}
	publishModelDelta(nil, &store.Turn{ID: "turn", ThreadID: "thread"}, event)

	for name, run := range map[string]func(*runtimeDeltaCaptureNotifier){
		"nil turn": func(notifier *runtimeDeltaCaptureNotifier) {
			publishModelDelta(notifier, nil, event)
		},
		"empty delta": func(notifier *runtimeDeltaCaptureNotifier) {
			publishModelDelta(notifier, &store.Turn{ID: "turn", ThreadID: "thread"}, core.ModelDeltaEvent{})
		},
	} {
		t.Run(name, func(t *testing.T) {
			notifier := &runtimeDeltaCaptureNotifier{}
			run(notifier)
			if notifier.method != "" || notifier.params != nil {
				t.Fatalf("unexpected notification %q %#v", notifier.method, notifier.params)
			}
		})
	}
}
