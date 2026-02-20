// Package tui provides a terminal user interface for debugging and
// interacting with gollem agents.
//
// The TUI displays the agent's execution in real-time, showing system prompts,
// user prompts, model responses, tool calls, and tool results with color-coded
// formatting. It includes a status bar with live usage statistics and supports
// keyboard navigation.
//
// # Usage
//
//	result, err := tui.DebugUI(agent, "Tell me about Tokyo")
//
// # Keyboard Controls
//
//   - q: quit
//   - s: step mode (pause between model calls)
//   - a: auto mode (run continuously)
//   - up/down: scroll through messages
package tui
