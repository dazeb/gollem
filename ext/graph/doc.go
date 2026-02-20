// Package graph provides a typed graph/workflow engine for complex multi-step
// agent workflows.
//
// A Graph represents a directed workflow where each node performs an action and
// returns the name of the next node to execute. The graph supports linear
// workflows, conditional branching, and cycles (with configurable iteration limits).
//
// Usage:
//
//	type MyState struct {
//	    Input  string
//	    Output string
//	    Step   int
//	}
//
//	g := graph.NewGraph[MyState]()
//	g.AddNode(graph.Node[MyState]{
//	    Name: "process",
//	    Run: func(ctx context.Context, state *MyState) (string, error) {
//	        state.Output = strings.ToUpper(state.Input)
//	        return "validate", nil // next node
//	    },
//	})
//	g.AddNode(graph.Node[MyState]{
//	    Name: "validate",
//	    Run: func(ctx context.Context, state *MyState) (string, error) {
//	        return "", nil // empty string = end
//	    },
//	})
//	g.SetEntryPoint("process")
//
//	result, err := g.Run(ctx, MyState{Input: "hello"})
package graph
