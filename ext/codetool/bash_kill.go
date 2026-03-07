package codetool

import (
	"context"

	"github.com/fugue-labs/gollem/core"
)

// BashKillParams are the parameters for the bash_kill tool.
type BashKillParams struct {
	ID string `json:"id" jsonschema:"description=Background process ID to kill (e.g. 'bg-1')"`
}

// BashKill creates a tool that kills a background process by ID.
func BashKill(opts ...Option) core.Tool {
	cfg := applyOpts(opts)

	return core.FuncTool[BashKillParams](
		"bash_kill",
		"Kill a background process by ID. Use bash_status(id='all') to list processes first.",
		func(ctx context.Context, params BashKillParams) (string, error) {
			if cfg.BackgroundProcessManager == nil {
				return "", &core.ModelRetryError{Message: "no background process manager available"}
			}
			if params.ID == "" {
				return "", &core.ModelRetryError{Message: "id is required — specify a process ID like 'bg-1'"}
			}
			return cfg.BackgroundProcessManager.Kill(params.ID)
		},
	)
}
