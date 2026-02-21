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
