package codetool

import (
	"context"
	"testing"

	"github.com/fugue-labs/gollem/core"
	traceutil "github.com/fugue-labs/gollem/ext/trace"
)

func TestSubAgentToolSharesEventBusForNestedReplayBoundaries(t *testing.T) {
	model := core.NewTestModel(
		core.ToolCallResponseWithID("delegate", `{"task":"answer from a child agent"}`, "delegate-call-1"),
		core.TextResponse("child result"),
		core.TextResponse("parent result"),
	)
	bus := core.NewEventBus()
	defer bus.Close()
	recorder := traceutil.NewRuntimeRecorder(bus)
	defer recorder.Close()

	agent := core.NewAgent[string](
		model,
		core.WithTools[string](SubAgentTool(model)),
		core.WithEventBus[string](bus),
		core.WithTracing[string](),
	)
	result, err := agent.Run(context.Background(), "delegate the work")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Trace == nil {
		t.Fatal("expected parent trace")
	}

	events := recorder.EventsForTrace(result.Trace.RunID)
	var childRunID string
	for _, event := range events {
		if event.Kind == "run.started" && event.AgentID != "" && event.CausalParentID == result.Trace.RunID {
			childRunID = event.AgentID
			break
		}
	}
	if childRunID == "" {
		t.Fatalf("expected nested child run event with parent %q, got %+v", result.Trace.RunID, events)
	}
	for _, want := range []string{"model.requested", "model.responded", "run.completed"} {
		if !hasRuntimeEvent(events, childRunID, want) {
			t.Fatalf("missing child event %s for %s in %+v", want, childRunID, events)
		}
	}
}

func hasRuntimeEvent(events []traceutil.Event, agentID, kind string) bool {
	for _, event := range events {
		if event.AgentID == agentID && event.Kind == kind {
			return true
		}
	}
	return false
}
