// Example graph demonstrates the graph workflow engine for building typed
// state machines. Each node in the graph performs an action and returns the
// name of the next node to execute, enabling conditional branching, loops,
// and complex multi-step workflows.
package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/trevorprater/gollem/graph"
)

// OrderState represents the state of an order processing workflow.
type OrderState struct {
	OrderID     string
	Items       []string
	TotalAmount float64
	Status      string
	Approved    bool
	Notes       []string
}

func main() {
	// Build a graph representing an order processing workflow.
	g := graph.NewGraph[OrderState]()

	// Node 1: Validate the order.
	g.AddNode(graph.Node[OrderState]{
		Name: "validate",
		Run: func(_ context.Context, s *OrderState) (string, error) {
			s.Notes = append(s.Notes, "Validating order...")

			if s.OrderID == "" {
				s.Status = "rejected"
				s.Notes = append(s.Notes, "Rejected: missing order ID")
				return graph.EndNode, nil
			}
			if len(s.Items) == 0 {
				s.Status = "rejected"
				s.Notes = append(s.Notes, "Rejected: no items")
				return graph.EndNode, nil
			}
			if s.TotalAmount <= 0 {
				s.Status = "rejected"
				s.Notes = append(s.Notes, "Rejected: invalid total")
				return graph.EndNode, nil
			}

			s.Notes = append(s.Notes, "Validation passed")

			// Orders over $1000 require approval.
			if s.TotalAmount > 1000 {
				return "approve", nil
			}
			return "process", nil
		},
	})

	// Node 2: Approval step for high-value orders.
	g.AddNode(graph.Node[OrderState]{
		Name: "approve",
		Run: func(_ context.Context, s *OrderState) (string, error) {
			s.Notes = append(s.Notes, fmt.Sprintf("High-value order ($%.2f) requires approval", s.TotalAmount))

			// Simulate approval logic.
			if s.TotalAmount < 5000 {
				s.Approved = true
				s.Notes = append(s.Notes, "Auto-approved (under $5000)")
				return "process", nil
			}

			s.Approved = false
			s.Status = "pending_review"
			s.Notes = append(s.Notes, "Requires manual review (over $5000)")
			return graph.EndNode, nil
		},
	})

	// Node 3: Process the order.
	g.AddNode(graph.Node[OrderState]{
		Name: "process",
		Run: func(_ context.Context, s *OrderState) (string, error) {
			s.Notes = append(s.Notes, "Processing order...")
			s.Status = "processing"

			// Simulate processing.
			s.Notes = append(s.Notes, fmt.Sprintf("Reserving %d items: %s",
				len(s.Items), strings.Join(s.Items, ", ")))

			return "ship", nil
		},
	})

	// Node 4: Ship the order.
	g.AddNode(graph.Node[OrderState]{
		Name: "ship",
		Run: func(_ context.Context, s *OrderState) (string, error) {
			s.Notes = append(s.Notes, "Shipping order...")
			s.Status = "shipped"
			s.Notes = append(s.Notes, fmt.Sprintf("Order %s shipped successfully", s.OrderID))
			return graph.EndNode, nil
		},
	})

	// Set the entry point and iteration limit.
	g.SetEntryPoint("validate")
	g.SetMaxIterations(10)

	// Print the graph as a Mermaid diagram.
	fmt.Println("=== Workflow Diagram (Mermaid) ===")
	fmt.Println(g.Mermaid())

	// --- Scenario 1: Normal order (auto-processed) ---
	fmt.Println("=== Scenario 1: Normal Order ($150) ===")
	state1, err := g.Run(context.Background(), OrderState{
		OrderID:     "ORD-001",
		Items:       []string{"Widget A", "Widget B"},
		TotalAmount: 150.00,
	})
	if err != nil {
		log.Fatal(err)
	}
	printState(state1)

	// --- Scenario 2: High-value order (requires approval, auto-approved) ---
	fmt.Println("=== Scenario 2: High-Value Order ($2500) ===")
	state2, err := g.Run(context.Background(), OrderState{
		OrderID:     "ORD-002",
		Items:       []string{"Premium Package", "Support Plan"},
		TotalAmount: 2500.00,
	})
	if err != nil {
		log.Fatal(err)
	}
	printState(state2)

	// --- Scenario 3: Very high-value order (requires manual review) ---
	fmt.Println("=== Scenario 3: Very High-Value Order ($7500) ===")
	state3, err := g.Run(context.Background(), OrderState{
		OrderID:     "ORD-003",
		Items:       []string{"Enterprise License"},
		TotalAmount: 7500.00,
	})
	if err != nil {
		log.Fatal(err)
	}
	printState(state3)

	// --- Scenario 4: Invalid order (no items) ---
	fmt.Println("=== Scenario 4: Invalid Order (no items) ===")
	state4, err := g.Run(context.Background(), OrderState{
		OrderID:     "ORD-004",
		TotalAmount: 50.00,
	})
	if err != nil {
		log.Fatal(err)
	}
	printState(state4)
}

func printState(s *OrderState) {
	fmt.Printf("  Order: %s\n", s.OrderID)
	fmt.Printf("  Status: %s\n", s.Status)
	fmt.Printf("  Approved: %v\n", s.Approved)
	fmt.Printf("  Notes:\n")
	for _, note := range s.Notes {
		fmt.Printf("    - %s\n", note)
	}
	fmt.Println()
}
