package codetool

import (
	"context"
	"strings"
	"sync"

	"github.com/fugue-labs/gollem/core"
)

// LoopDetectionMiddleware detects when the model is stuck in a doom loop —
// repeatedly making similar tool calls without progress. After threshold
// repeated edits to the same file, it injects a message telling the model
// to reconsider its approach.
func LoopDetectionMiddleware(threshold int) core.AgentMiddleware {
	if threshold <= 0 {
		threshold = 3
	}

	var mu sync.Mutex
	editCounts := make(map[string]int) // file path -> consecutive edit count

	return func(
		ctx context.Context,
		messages []core.ModelMessage,
		settings *core.ModelSettings,
		params *core.ModelRequestParameters,
		next func(context.Context, []core.ModelMessage, *core.ModelSettings, *core.ModelRequestParameters) (*core.ModelResponse, error),
	) (*core.ModelResponse, error) {
		// Check the last message for repeated edits.
		mu.Lock()
		if len(messages) > 0 {
			if req, ok := messages[len(messages)-1].(core.ModelRequest); ok {
				for _, part := range req.Parts {
					if tr, ok := part.(core.ToolReturnPart); ok {
						if tr.ToolName == "edit" || tr.ToolName == "multi_edit" {
							// Extract file path from the return content.
							content, _ := tr.Content.(string)
							for path, count := range editCounts {
								if strings.Contains(content, path) {
									editCounts[path] = count + 1
								}
							}
							// Track new paths mentioned in the result.
							if strings.Contains(content, "Replaced") || strings.Contains(content, "edited") {
								// Parse path from result like "Replaced 1 occurrence(s) in foo.go"
								parts := strings.Fields(content)
								for _, p := range parts {
									if strings.Contains(p, ".") && !strings.HasPrefix(p, "occurrence") {
										if _, exists := editCounts[p]; !exists {
											editCounts[p] = 1
										}
									}
								}
							}
						}
					}
				}
			}
		}

		// Check if any file has been edited too many times.
		var loopedFiles []string
		for path, count := range editCounts {
			if count >= threshold {
				loopedFiles = append(loopedFiles, path)
			}
		}
		mu.Unlock()

		if len(loopedFiles) > 0 {
			// Inject a guidance message.
			loopMsg := core.ModelRequest{
				Parts: []core.ModelRequestPart{
					core.UserPromptPart{
						Content: "WARNING: You appear to be stuck in a loop, repeatedly editing " +
							strings.Join(loopedFiles, ", ") + ". " +
							"Step back and reconsider your approach. Try a different strategy: " +
							"re-read the file, check error messages carefully, or try a completely different solution path.",
					},
				},
			}
			messages = append(messages, loopMsg)

			// Reset counts so we don't keep injecting.
			mu.Lock()
			for _, path := range loopedFiles {
				delete(editCounts, path)
			}
			mu.Unlock()
		}

		return next(ctx, messages, settings, params)
	}
}

// ContextInjectionMiddleware injects environment context at the start of the
// conversation. It runs a set of bash commands to discover the environment
// (e.g., directory structure, language, available tools) and prepends the
// results as a system-level context message.
func ContextInjectionMiddleware(workDir string) core.AgentMiddleware {
	var once sync.Once
	var envContext string

	return func(
		ctx context.Context,
		messages []core.ModelMessage,
		settings *core.ModelSettings,
		params *core.ModelRequestParameters,
		next func(context.Context, []core.ModelMessage, *core.ModelSettings, *core.ModelRequestParameters) (*core.ModelResponse, error),
	) (*core.ModelResponse, error) {
		once.Do(func() {
			envContext = discoverEnvironment(workDir)
		})

		if envContext != "" && len(messages) > 0 {
			// Inject environment context into the first request's system parts.
			if req, ok := messages[0].(core.ModelRequest); ok {
				envPart := core.SystemPromptPart{
					Content: envContext,
				}
				newParts := make([]core.ModelRequestPart, 0, len(req.Parts)+1)
				newParts = append(newParts, envPart)
				newParts = append(newParts, req.Parts...)
				req.Parts = newParts
				messages[0] = req
			}
		}

		return next(ctx, messages, settings, params)
	}
}

// discoverEnvironment runs lightweight discovery commands to map the workspace.
func discoverEnvironment(workDir string) string {
	var parts []string
	parts = append(parts, "## Environment Context")
	parts = append(parts, "Working directory: "+workDir)

	// These are static context hints. The actual discovery happens when the
	// agent uses ls/grep/bash tools at runtime. This gives it a head start.
	parts = append(parts, "You can use bash, view, edit, write, grep, glob, and ls tools.")
	parts = append(parts, "Start by using ls to understand the project structure, then proceed with the task.")

	return strings.Join(parts, "\n")
}
