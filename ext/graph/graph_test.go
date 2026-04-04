package graph

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
)

type testState struct {
	Input   string
	Output  string
	Counter int
	Path    []string
}

func TestGraph_LinearWorkflow(t *testing.T) {
	g := NewGraph[testState]()
	g.AddNode(Node[testState]{
		Name: "step1",
		Run: func(_ context.Context, s *testState) (string, error) {
			s.Path = append(s.Path, "step1")
			s.Output = "processed: " + s.Input
			return "step2", nil
		},
	})
	g.AddNode(Node[testState]{
		Name: "step2",
		Run: func(_ context.Context, s *testState) (string, error) {
			s.Path = append(s.Path, "step2")
			s.Output = strings.ToUpper(s.Output)
			return "step3", nil
		},
	})
	g.AddNode(Node[testState]{
		Name: "step3",
		Run: func(_ context.Context, s *testState) (string, error) {
			s.Path = append(s.Path, "step3")
			return "", nil // end
		},
	})
	g.SetEntryPoint("step1")

	result, err := g.Run(context.Background(), testState{Input: "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "PROCESSED: HELLO" {
		t.Errorf("unexpected output: %q", result.Output)
	}
	if len(result.Path) != 3 || result.Path[0] != "step1" || result.Path[1] != "step2" || result.Path[2] != "step3" {
		t.Errorf("unexpected path: %v", result.Path)
	}
}

func TestGraph_ConditionalBranching(t *testing.T) {
	g := NewGraph[testState]()
	g.AddNode(Node[testState]{
		Name: "check",
		Run: func(_ context.Context, s *testState) (string, error) {
			s.Path = append(s.Path, "check")
			if s.Input == "go-left" {
				return "left", nil
			}
			return "right", nil
		},
	})
	g.AddNode(Node[testState]{
		Name: "left",
		Run: func(_ context.Context, s *testState) (string, error) {
			s.Path = append(s.Path, "left")
			s.Output = "went left"
			return "", nil
		},
	})
	g.AddNode(Node[testState]{
		Name: "right",
		Run: func(_ context.Context, s *testState) (string, error) {
			s.Path = append(s.Path, "right")
			s.Output = "went right"
			return "", nil
		},
	})
	g.SetEntryPoint("check")

	// Test left path.
	result, err := g.Run(context.Background(), testState{Input: "go-left"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "went left" {
		t.Errorf("expected 'went left', got %q", result.Output)
	}

	// Test right path.
	result, err = g.Run(context.Background(), testState{Input: "go-right"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "went right" {
		t.Errorf("expected 'went right', got %q", result.Output)
	}
}

func TestGraph_CycleDetection(t *testing.T) {
	g := NewGraph[testState]()
	g.AddNode(Node[testState]{
		Name: "loop",
		Run: func(_ context.Context, s *testState) (string, error) {
			s.Counter++
			return "loop", nil // infinite loop
		},
	})
	g.SetEntryPoint("loop")
	g.SetMaxIterations(10)

	result, err := g.Run(context.Background(), testState{})
	if err == nil {
		t.Fatal("expected max iterations error")
	}
	if !strings.Contains(err.Error(), "max iterations") {
		t.Errorf("unexpected error: %v", err)
	}
	if result.Counter != 10 {
		t.Errorf("expected 10 iterations, got %d", result.Counter)
	}
}

func TestGraph_MermaidGeneration(t *testing.T) {
	g := NewGraph[testState]()
	g.AddNode(Node[testState]{Name: "A", Run: func(_ context.Context, _ *testState) (string, error) { return "B", nil }})
	g.AddNode(Node[testState]{Name: "B", Run: func(_ context.Context, _ *testState) (string, error) { return "", nil }})
	g.SetEntryPoint("A")

	mermaid := g.Mermaid()
	if !strings.Contains(mermaid, "graph TD") {
		t.Error("expected 'graph TD' in Mermaid output")
	}
	if !strings.Contains(mermaid, "A") {
		t.Error("expected node A in Mermaid output")
	}
	if !strings.Contains(mermaid, "B") {
		t.Error("expected node B in Mermaid output")
	}
	if !strings.Contains(mermaid, "START") {
		t.Error("expected START marker in Mermaid output")
	}
}

func TestGraph_NoEntryPoint(t *testing.T) {
	g := NewGraph[testState]()
	g.AddNode(Node[testState]{Name: "A", Run: func(_ context.Context, _ *testState) (string, error) { return "", nil }})

	_, err := g.Run(context.Background(), testState{})
	if err == nil {
		t.Fatal("expected error for no entry point")
	}
}

func TestGraph_MissingNode(t *testing.T) {
	g := NewGraph[testState]()
	g.AddNode(Node[testState]{
		Name: "A",
		Run: func(_ context.Context, _ *testState) (string, error) {
			return "nonexistent", nil
		},
	})
	g.SetEntryPoint("A")

	_, err := g.Run(context.Background(), testState{})
	if err == nil {
		t.Fatal("expected error for missing node")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGraph_EndNode(t *testing.T) {
	g := NewGraph[testState]()
	g.AddNode(Node[testState]{
		Name: "A",
		Run: func(_ context.Context, s *testState) (string, error) {
			s.Output = "done"
			return EndNode, nil // explicit end
		},
	})
	g.SetEntryPoint("A")

	result, err := g.Run(context.Background(), testState{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "done" {
		t.Errorf("expected 'done', got %q", result.Output)
	}
}

func TestGraph_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	g := NewGraph[testState]()
	g.AddNode(Node[testState]{
		Name: "A",
		Run: func(_ context.Context, _ *testState) (string, error) {
			return "B", nil
		},
	})
	g.SetEntryPoint("A")

	_, err := g.Run(ctx, testState{})
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestGraph_FanOut_ParallelExecution(t *testing.T) {
	var execCount atomic.Int32

	g := NewGraph[testState]()

	// Add three worker nodes that each record their execution.
	g.AddNode(Node[testState]{
		Name: "worker_a",
		Run: func(_ context.Context, s *testState) (string, error) {
			execCount.Add(1)
			s.Output = "a"
			s.Path = []string{"worker_a"}
			return "", nil
		},
	})
	g.AddNode(Node[testState]{
		Name: "worker_b",
		Run: func(_ context.Context, s *testState) (string, error) {
			execCount.Add(1)
			s.Output = "b"
			s.Path = []string{"worker_b"}
			return "", nil
		},
	})
	g.AddNode(Node[testState]{
		Name: "worker_c",
		Run: func(_ context.Context, s *testState) (string, error) {
			execCount.Add(1)
			s.Output = "c"
			s.Path = []string{"worker_c"}
			return "", nil
		},
	})

	// Add a final node to verify continuation after fan-out.
	g.AddNode(Node[testState]{
		Name: "final",
		Run: func(_ context.Context, s *testState) (string, error) {
			s.Path = append(s.Path, "final")
			return EndNode, nil
		},
	})

	// Add fan-out node that sends to all three workers.
	g.AddFanOutNode("dispatch", func(_ context.Context, s *testState) ([]Send[testState], string, error) {
		return []Send[testState]{
			{Node: "worker_a", State: *s},
			{Node: "worker_b", State: *s},
			{Node: "worker_c", State: *s},
		}, "final", nil
	})

	// Reducer that merges paths from all branches.
	g.SetReducer(func(states []testState) testState {
		var merged testState
		for _, s := range states {
			merged.Path = append(merged.Path, s.Path...)
		}
		sort.Strings(merged.Path)
		return merged
	})

	g.SetEntryPoint("dispatch")

	result, err := g.Run(context.Background(), testState{Input: "start"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All three workers must have executed.
	if execCount.Load() != 3 {
		t.Errorf("expected 3 executions, got %d", execCount.Load())
	}

	// The path should contain all three worker names plus "final".
	if len(result.Path) != 4 {
		t.Fatalf("expected 4 path entries, got %d: %v", len(result.Path), result.Path)
	}
	// After sort+append of "final", path should be [final, worker_a, worker_b, worker_c].
	// Actually, reducer produces sorted [worker_a, worker_b, worker_c], then "final" is appended.
	expected := []string{"worker_a", "worker_b", "worker_c", "final"}
	for i, want := range expected {
		if result.Path[i] != want {
			t.Errorf("path[%d] = %q, want %q", i, result.Path[i], want)
		}
	}
}

func TestGraph_FanOut_Reduce(t *testing.T) {
	g := NewGraph[testState]()

	// Three worker nodes that each set Counter to a different value.
	g.AddNode(Node[testState]{
		Name: "add10",
		Run: func(_ context.Context, s *testState) (string, error) {
			s.Counter = 10
			return "", nil
		},
	})
	g.AddNode(Node[testState]{
		Name: "add20",
		Run: func(_ context.Context, s *testState) (string, error) {
			s.Counter = 20
			return "", nil
		},
	})
	g.AddNode(Node[testState]{
		Name: "add30",
		Run: func(_ context.Context, s *testState) (string, error) {
			s.Counter = 30
			return "", nil
		},
	})

	g.AddFanOutNode("scatter", func(_ context.Context, s *testState) ([]Send[testState], string, error) {
		return []Send[testState]{
			{Node: "add10", State: *s},
			{Node: "add20", State: *s},
			{Node: "add30", State: *s},
		}, EndNode, nil
	})

	// Reducer sums all counters.
	g.SetReducer(func(states []testState) testState {
		var merged testState
		for _, s := range states {
			merged.Counter += s.Counter
		}
		return merged
	})

	g.SetEntryPoint("scatter")

	result, err := g.Run(context.Background(), testState{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Counter != 60 {
		t.Errorf("expected Counter=60, got %d", result.Counter)
	}
}

func TestGraph_FanOut_ErrorPropagation(t *testing.T) {
	g := NewGraph[testState]()

	g.AddNode(Node[testState]{
		Name: "ok_node",
		Run: func(_ context.Context, s *testState) (string, error) {
			s.Output = "ok"
			return "", nil
		},
	})
	g.AddNode(Node[testState]{
		Name: "fail_node",
		Run: func(_ context.Context, _ *testState) (string, error) {
			return "", errors.New("branch failed")
		},
	})

	g.AddFanOutNode("scatter", func(_ context.Context, s *testState) ([]Send[testState], string, error) {
		return []Send[testState]{
			{Node: "ok_node", State: *s},
			{Node: "fail_node", State: *s},
		}, "done", nil
	})

	g.SetReducer(func(states []testState) testState {
		return states[0]
	})

	g.SetEntryPoint("scatter")

	_, err := g.Run(context.Background(), testState{})
	if err == nil {
		t.Fatal("expected error from failed branch")
	}
	if !strings.Contains(err.Error(), "branch failed") {
		t.Errorf("expected 'branch failed' in error, got: %v", err)
	}
}

func TestGraph_FanOut_EmptySends(t *testing.T) {
	g := NewGraph[testState]()

	// Fan-out that produces zero sends.
	g.AddFanOutNode("empty_scatter", func(_ context.Context, _ *testState) ([]Send[testState], string, error) {
		return nil, "collect", nil
	})

	g.AddNode(Node[testState]{
		Name: "collect",
		Run: func(_ context.Context, s *testState) (string, error) {
			s.Output = "reached collect"
			return EndNode, nil
		},
	})

	g.SetEntryPoint("empty_scatter")

	result, err := g.Run(context.Background(), testState{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "reached collect" {
		t.Errorf("expected 'reached collect', got %q", result.Output)
	}
}

func TestGraph_FanOut_Mermaid(t *testing.T) {
	g := NewGraph[testState]()

	g.AddNode(Node[testState]{
		Name: "start",
		Run:  func(_ context.Context, _ *testState) (string, error) { return "scatter", nil },
	})
	g.AddFanOutNode("scatter", func(_ context.Context, _ *testState) ([]Send[testState], string, error) {
		return nil, EndNode, nil
	})
	g.AddNode(Node[testState]{
		Name: "collect",
		Run:  func(_ context.Context, _ *testState) (string, error) { return EndNode, nil },
	})
	g.SetEntryPoint("start")

	mermaid := g.Mermaid()

	if !strings.Contains(mermaid, "graph TD") {
		t.Error("expected 'graph TD' in Mermaid output")
	}
	if !strings.Contains(mermaid, "Fan-Out") {
		t.Error("expected 'Fan-Out' label in Mermaid output")
	}
	// Fan-out nodes use stadium shape ([" ... "]).
	if !strings.Contains(mermaid, "scatter([") {
		t.Error("expected stadium shape for fan-out node in Mermaid output")
	}
	// Regular nodes use rectangle shape.
	if !strings.Contains(mermaid, "start[") {
		t.Error("expected rectangle shape for regular node in Mermaid output")
	}
	if !strings.Contains(mermaid, "collect[") {
		t.Error("expected rectangle shape for collect node in Mermaid output")
	}
}
