// Package memory provides conversation memory implementations for gollem
// agents, allowing message history to be stored and retrieved across runs.
//
// # Usage
//
//	buf := memory.NewBuffer(memory.WithMaxMessages(50))
//	buf.Add(ctx, messages...)
//	history, _ := buf.Get(ctx)
//
// BufferMemory is an in-memory circular buffer that drops the oldest
// messages when the maximum size is exceeded. It is thread-safe.
package memory
