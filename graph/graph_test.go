package graph

import (
	"context"
	"strings"
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
