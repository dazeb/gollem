package graph

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

// Graph represents a workflow as a directed graph of nodes.
type Graph[S any] struct {
	nodes         map[string]Node[S]
	entryPoint    string
	maxIterations int
	nodeOrder     []string // tracks insertion order for Mermaid
}

// NewGraph creates a new workflow graph with the given state type.
func NewGraph[S any]() *Graph[S] {
	return &Graph[S]{
		nodes:         make(map[string]Node[S]),
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

// Run executes the graph from the entry point to completion.
func (g *Graph[S]) Run(ctx context.Context, initialState S) (*S, error) {
	if g.entryPoint == "" {
		return nil, errors.New("no entry point set")
	}
	if _, ok := g.nodes[g.entryPoint]; !ok {
		return nil, fmt.Errorf("entry point %q not found in graph", g.entryPoint)
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

// Mermaid returns a Mermaid diagram of the graph.
// Since the graph is dynamic (next node depends on runtime state), this
// generates a simple node listing with the entry point marked.
func (g *Graph[S]) Mermaid() string {
	var sb strings.Builder
	sb.WriteString("graph TD\n")

	for _, name := range g.nodeOrder {
		label := name
		if name == g.entryPoint {
			label = name + " [Start]"
		}
		sb.WriteString(fmt.Sprintf("    %s[\"%s\"]\n", name, label))
	}

	// Add entry point arrow.
	if g.entryPoint != "" {
		sb.WriteString(fmt.Sprintf("    START(( )) --> %s\n", g.entryPoint))
	}

	return sb.String()
}
