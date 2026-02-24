package codetool

import (
	"context"
	"testing"

	"github.com/fugue-labs/gollem/core"
)

func TestIsCodeModeFailure(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name:    "module not found",
			content: "ModuleNotFoundError: No module named 'numpy'",
			want:    true,
		},
		{
			name:    "monty parser limitation",
			content: "NotImplementedError: The monty syntax parser does not yet support context managers",
			want:    true,
		},
		{
			name:    "missing open builtin",
			content: "NameError: name 'open' is not defined",
			want:    true,
		},
		{
			name:    "normal runtime error",
			content: "ValueError: invalid literal for int() with base 10: 'abc'",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isCodeModeFailure(tt.content)
			if got != tt.want {
				t.Fatalf("isCodeModeFailure(%q) = %v, want %v", tt.content, got, tt.want)
			}
		})
	}
}

func TestShouldTemporarilyDisableExecuteCode(t *testing.T) {
	cfg := codeModeFallbackConfig{
		failureThreshold: 3,
		cooldownTurns:    2,
		maxRecentResults: 12,
	}

	t.Run("disable after three consecutive capability failures", func(t *testing.T) {
		msgs := []core.ModelMessage{
			executeCodeToolReturn("ModuleNotFoundError: No module named 'numpy'"),
			assistantTurn(),
			executeCodeToolReturn("NotImplementedError: The monty syntax parser does not yet support context managers"),
			assistantTurn(),
			executeCodeToolReturn("ImportError: cannot import name 'pathlib2'"),
		}

		disable, _ := shouldTemporarilyDisableExecuteCode(msgs, cfg)
		if !disable {
			t.Fatal("expected execute_code to be temporarily disabled")
		}
	})

	t.Run("cooldown keeps it disabled for one turn", func(t *testing.T) {
		msgs := []core.ModelMessage{
			executeCodeToolReturn("ModuleNotFoundError: No module named 'numpy'"),
			assistantTurn(),
			executeCodeToolReturn("NotImplementedError: The monty syntax parser does not yet support context managers"),
			assistantTurn(),
			executeCodeToolReturn("ImportError: cannot import name 'pathlib2'"),
			assistantTurn(),
		}

		disable, _ := shouldTemporarilyDisableExecuteCode(msgs, cfg)
		if !disable {
			t.Fatal("expected execute_code to remain disabled during cooldown")
		}
	})

	t.Run("reenable after cooldown expires", func(t *testing.T) {
		msgs := []core.ModelMessage{
			executeCodeToolReturn("ModuleNotFoundError: No module named 'numpy'"),
			assistantTurn(),
			executeCodeToolReturn("NotImplementedError: The monty syntax parser does not yet support context managers"),
			assistantTurn(),
			executeCodeToolReturn("ImportError: cannot import name 'pathlib2'"),
			assistantTurn(),
			assistantTurn(),
		}

		disable, _ := shouldTemporarilyDisableExecuteCode(msgs, cfg)
		if disable {
			t.Fatal("expected execute_code to be re-enabled after cooldown")
		}
	})

	t.Run("cooldown gap resets failure streak", func(t *testing.T) {
		msgs := []core.ModelMessage{
			executeCodeToolReturn("ModuleNotFoundError: No module named 'numpy'"),
			assistantTurn(),
			executeCodeToolReturn("NotImplementedError: The monty syntax parser does not yet support context managers"),
			assistantTurn(),
			executeCodeToolReturn("ImportError: cannot import name 'pathlib2'"),
			assistantTurn(),
			assistantTurn(),
			executeCodeToolReturn("ModuleNotFoundError: No module named 'pandas'"),
		}

		disable, _ := shouldTemporarilyDisableExecuteCode(msgs, cfg)
		if disable {
			t.Fatal("expected single post-cooldown failure to not disable execute_code")
		}
	})

	t.Run("runtime errors do not count as capability failures", func(t *testing.T) {
		msgs := []core.ModelMessage{
			executeCodeToolReturn("ValueError: bad input"),
			assistantTurn(),
			executeCodeToolReturn("TypeError: unsupported operand type"),
			assistantTurn(),
			executeCodeToolReturn("KeyError: 'missing'"),
		}

		disable, _ := shouldTemporarilyDisableExecuteCode(msgs, cfg)
		if disable {
			t.Fatal("expected execute_code to remain enabled for non-capability errors")
		}
	})
}

func TestDisableExecuteCodeOnImportFailuresPrepare(t *testing.T) {
	prepare := disableExecuteCodeOnImportFailuresPrepare()

	tools := []core.ToolDefinition{
		{Name: "bash"},
		{Name: "execute_code"},
		{Name: "edit"},
	}

	rc := &core.RunContext{
		Messages: []core.ModelMessage{
			executeCodeToolReturn("ModuleNotFoundError: No module named 'numpy'"),
			assistantTurn(),
			executeCodeToolReturn("NotImplementedError: The monty syntax parser does not yet support with statements"),
			assistantTurn(),
			executeCodeToolReturn("ImportError: cannot import name 'pathlib2'"),
		},
	}

	filtered := prepare(context.Background(), rc, tools)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 tools after filtering, got %d", len(filtered))
	}
	for _, tool := range filtered {
		if tool.Name == "execute_code" {
			t.Fatal("execute_code should be temporarily removed during cooldown")
		}
	}

	rc.Messages = append(rc.Messages, assistantTurn(), assistantTurn())
	filtered = prepare(context.Background(), rc, tools)
	for _, tool := range filtered {
		if tool.Name == "execute_code" {
			return
		}
	}
	t.Fatal("execute_code should be re-enabled after cooldown turns")
}

func TestLoadCodeModeFallbackConfigFromEnv(t *testing.T) {
	t.Setenv("GOLLEM_CODE_MODE_FAILURE_THRESHOLD", "4")
	t.Setenv("GOLLEM_CODE_MODE_COOLDOWN_TURNS", "3")
	t.Setenv("GOLLEM_CODE_MODE_MAX_RECENT_RESULTS", "2")

	cfg := loadCodeModeFallbackConfig()
	if cfg.failureThreshold != 4 {
		t.Fatalf("failureThreshold = %d, want 4", cfg.failureThreshold)
	}
	if cfg.cooldownTurns != 3 {
		t.Fatalf("cooldownTurns = %d, want 3", cfg.cooldownTurns)
	}
	// maxRecentResults is clamped up to failureThreshold.
	if cfg.maxRecentResults != 4 {
		t.Fatalf("maxRecentResults = %d, want 4", cfg.maxRecentResults)
	}
}

func executeCodeToolReturn(content string) core.ModelMessage {
	return core.ModelRequest{
		Parts: []core.ModelRequestPart{
			core.ToolReturnPart{
				ToolName: "execute_code",
				Content:  content,
			},
		},
	}
}

func assistantTurn() core.ModelMessage {
	return core.ModelResponse{}
}
