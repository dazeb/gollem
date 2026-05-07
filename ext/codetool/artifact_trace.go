package codetool

import (
	"context"
	"os"
	"time"

	"github.com/fugue-labs/gollem/core"
)

func writeFileAndTrace(ctx context.Context, rc *core.RunContext, cfg *Config, path string, content []byte, perm os.FileMode, operation, toolName string) error {
	if err := os.WriteFile(path, content, perm); err != nil {
		return err
	}
	publishArtifactChanged(ctx, rc, cfg, path, operation, toolName, int64(len(content)))
	return nil
}

func publishArtifactChanged(ctx context.Context, rc *core.RunContext, cfg *Config, path, operation, toolName string, bytes int64) {
	bus := (*core.EventBus)(nil)
	if cfg != nil {
		bus = cfg.EventBus
	}
	if bus == nil && rc != nil {
		bus = rc.EventBus
	}
	if bus == nil {
		return
	}

	runID := core.RunIDFromContext(ctx)
	toolCallID := core.ToolCallIDFromContext(ctx)
	if rc != nil {
		if runID == "" {
			runID = rc.RunID
		}
		if toolCallID == "" {
			toolCallID = rc.ToolCallID
		}
		if toolName == "" {
			toolName = rc.ToolName
		}
	}

	var parentRunID string
	if rc != nil {
		parentRunID = rc.ParentRunID
	}

	core.Publish(bus, core.ArtifactChangedEvent{
		RunID:       runID,
		ParentRunID: parentRunID,
		ToolCallID:  toolCallID,
		ToolName:    toolName,
		Path:        path,
		Operation:   operation,
		Bytes:       bytes,
		ChangedAt:   time.Now(),
	})
}
