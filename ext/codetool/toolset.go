package codetool

import (
	"github.com/fugue-labs/gollem/core"
)

// Toolset returns all coding agent tools as a core.Toolset.
// Use this to add the full suite of coding tools to an agent:
//
//	ts := codetool.Toolset(codetool.WithWorkDir("/my/project"))
//	agent := core.NewAgent(model, "...", core.WithToolset(ts))
func Toolset(opts ...Option) *core.Toolset {
	return core.NewToolset("codetool",
		Bash(opts...),
		View(opts...),
		Write(opts...),
		Edit(opts...),
		MultiEdit(opts...),
		Grep(opts...),
		Glob(opts...),
		Ls(opts...),
	)
}

// AllTools returns all coding agent tools as a slice.
func AllTools(opts ...Option) []core.Tool {
	return []core.Tool{
		Bash(opts...),
		View(opts...),
		Write(opts...),
		Edit(opts...),
		MultiEdit(opts...),
		Grep(opts...),
		Glob(opts...),
		Ls(opts...),
	}
}

// AgentOptions returns the recommended set of agent options for a coding agent.
// This includes the system prompt, toolset, middleware (loop detection, context
// injection, verification checkpoint), and the output validator.
//
// Usage:
//
//	opts := codetool.AgentOptions("/path/to/project")
//	agent := core.NewAgent[string](model, opts...)
func AgentOptions(workDir string, toolOpts ...Option) []core.AgentOption[string] {
	if workDir != "" {
		toolOpts = append([]Option{WithWorkDir(workDir)}, toolOpts...)
	}
	verifyMW, verifyValidator := VerificationCheckpoint()
	return []core.AgentOption[string]{
		core.WithSystemPrompt[string](SystemPrompt),
		core.WithToolsets[string](Toolset(toolOpts...)),
		core.WithMaxRetries[string](3),
		core.WithAgentMiddleware[string](LoopDetectionMiddleware(4)),
		core.WithAgentMiddleware[string](ContextInjectionMiddleware(workDir)),
		core.WithAgentMiddleware[string](verifyMW),
		core.WithOutputValidator[string](verifyValidator),
	}
}
