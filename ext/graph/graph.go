package graph

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

const (
	// EndNode is the sentinel value returned by nodes to terminate the graph.
	EndNode = "__end__"

	// DefaultMaxIterations is the default maximum number of node executions.
	DefaultMaxIterations = 100
)

// Node represents a single step in the workflow.
type Node[S any] struct {
	// Name is the unique identifier for this node.
	Name string
	// Run executes the node's logic. It receives the current state and returns
	// the name of the next node to execute. Return "" or EndNode to terminate.
	Run func(ctx context.Context, state *S) (string, error)
}

// Send represents a directive to execute a specific node with specific state.
type Send[S any] struct {
	Node  string
	State S
}

// FanOutFunc returns Send directives for parallel execution and the name of
// the node to continue to after reduce.
type FanOutFunc[S any] func(ctx context.Context, state *S) (sends []Send[S], continueNode string, err error)

// ReduceFunc merges results from parallel branches into a single state.
type ReduceFunc[S any] func(states []S) S

// Graph represents a workflow as a directed graph of nodes.
type Graph[S any] struct {
	nodes         map[string]Node[S]
	fanOutNodes   map[string]FanOutFunc[S]
	reducer       ReduceFunc[S]
	entryPoint    string
	maxIterations int
	nodeOrder     []string // tracks insertion order for Mermaid
}

// NewGraph creates a new workflow graph with the given state type.
func NewGraph[S any]() *Graph[S] {
	return &Graph[S]{
		nodes:         make(map[string]Node[S]),
		fanOutNodes:   make(map[string]FanOutFunc[S]),
		maxIterations: DefaultMaxIterations,
	}
}

// AddNode adds a named node to the graph.
func (g *Graph[S]) AddNode(node Node[S]) *Graph[S] {
	g.nodes[node.Name] = node
	g.nodeOrder = append(g.nodeOrder, node.Name)
	return g
}

// SetEntryPoint sets the starting node.
func (g *Graph[S]) SetEntryPoint(name string) *Graph[S] {
	g.entryPoint = name
	return g
}

// SetMaxIterations sets the maximum number of node executions (cycle protection).
func (g *Graph[S]) SetMaxIterations(n int) *Graph[S] {
	g.maxIterations = n
	return g
}

// AddFanOutNode adds a fan-out node that spawns parallel branches.
func (g *Graph[S]) AddFanOutNode(name string, fn FanOutFunc[S]) *Graph[S] {
	g.fanOutNodes[name] = fn
	g.nodeOrder = append(g.nodeOrder, name)
	return g
}

// SetReducer sets the function used to merge fan-out results.
func (g *Graph[S]) SetReducer(fn ReduceFunc[S]) *Graph[S] {
	g.reducer = fn
	return g
}

// Run executes the graph from the entry point to completion.
func (g *Graph[S]) Run(ctx context.Context, initialState S) (*S, error) {
	if g.entryPoint == "" {
		return nil, errors.New("no entry point set")
	}
	if _, ok := g.nodes[g.entryPoint]; !ok {
		if _, ok := g.fanOutNodes[g.entryPoint]; !ok {
			return nil, fmt.Errorf("entry point %q not found in graph", g.entryPoint)
		}
	}

	state := initialState
	currentNode := g.entryPoint
	iterations := 0

	for {
		if err := ctx.Err(); err != nil {
			return &state, err
		}

		iterations++
		if iterations > g.maxIterations {
			return &state, fmt.Errorf("max iterations (%d) exceeded at node %q", g.maxIterations, currentNode)
		}

		// Check if the current node is a fan-out node.
		if fanOutFn, ok := g.fanOutNodes[currentNode]; ok {
			nextNode, err := g.executeFanOut(ctx, &state, currentNode, fanOutFn)
			if err != nil {
				return &state, err
			}

			// Check for termination.
			if nextNode == "" || nextNode == EndNode {
				return &state, nil
			}

			currentNode = nextNode
			continue
		}

		node, ok := g.nodes[currentNode]
		if !ok {
			return &state, fmt.Errorf("node %q not found", currentNode)
		}

		nextNode, err := node.Run(ctx, &state)
		if err != nil {
			return &state, fmt.Errorf("node %q failed: %w", currentNode, err)
		}

		// Check for termination.
		if nextNode == "" || nextNode == EndNode {
			return &state, nil
		}

		currentNode = nextNode
	}
}

// executeFanOut handles the execution of a fan-out node: it calls the fan-out
// function, executes each Send directive concurrently, collects results, and
// applies the reducer.
func (g *Graph[S]) executeFanOut(ctx context.Context, state *S, name string, fn FanOutFunc[S]) (string, error) {
	sends, continueNode, err := fn(ctx, state)
	if err != nil {
		return "", fmt.Errorf("fan-out node %q failed: %w", name, err)
	}

	// If there are no sends, skip directly to the continue node.
	if len(sends) == 0 {
		return continueNode, nil
	}

	// Verify reducer is set when there are sends to process.
	if g.reducer == nil {
		return "", fmt.Errorf("fan-out node %q produced sends but no reducer is set", name)
	}

	// Execute each Send directive concurrently.
	type result struct {
		state S
		err   error
	}

	results := make([]result, len(sends))
	var wg sync.WaitGroup
	wg.Add(len(sends))

	for i, send := range sends {
		go func(idx int, s Send[S]) {
			defer wg.Done()

			node, ok := g.nodes[s.Node]
			if !ok {
				results[idx] = result{err: fmt.Errorf("fan-out target node %q not found", s.Node)}
				return
			}

			branchState := s.State
			_, err := node.Run(ctx, &branchState)
			if err != nil {
				results[idx] = result{err: fmt.Errorf("fan-out branch node %q failed: %w", s.Node, err)}
				return
			}

			results[idx] = result{state: branchState}
		}(i, send)
	}

	wg.Wait()

	// Check for errors from any branch.
	var errs []error
	var states []S
	for _, r := range results {
		if r.err != nil {
			errs = append(errs, r.err)
		} else {
			states = append(states, r.state)
		}
	}

	if len(errs) > 0 {
		return "", errors.Join(errs...)
	}

	// Apply the reducer to merge branch states.
	*state = g.reducer(states)

	return continueNode, nil
}

// Mermaid returns a Mermaid diagram of the graph.
// Since the graph is dynamic (next node depends on runtime state), this
// generates a simple node listing with the entry point marked.
// Fan-out nodes are rendered with a stadium shape (rounded edges) to
// distinguish them from regular nodes.
func (g *Graph[S]) Mermaid() string {
	var sb strings.Builder
	sb.WriteString("graph TD\n")

	for _, name := range g.nodeOrder {
		if _, isFanOut := g.fanOutNodes[name]; isFanOut {
			label := name + " [Fan-Out]"
			if name == g.entryPoint {
				label = name + " [Start, Fan-Out]"
			}
			sb.WriteString(fmt.Sprintf("    %s([\"%s\"])\n", name, label))
		} else {
			label := name
			if name == g.entryPoint {
				label = name + " [Start]"
			}
			sb.WriteString(fmt.Sprintf("    %s[\"%s\"]\n", name, label))
		}
	}

	// Add entry point arrow.
	if g.entryPoint != "" {
		sb.WriteString(fmt.Sprintf("    START(( )) --> %s\n", g.entryPoint))
	}

	return sb.String()
}
