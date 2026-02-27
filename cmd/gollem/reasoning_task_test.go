package main

import "testing"

func TestParseReasoningByTask_HandlesQuotedValue(t *testing.T) {
	got := parseReasoningByTask("'model-extraction-relu-logits=xhigh,*=high'")
	if got["model-extraction-relu-logits"] != "xhigh" {
		t.Fatalf("expected model-extraction-relu-logits=xhigh, got %q", got["model-extraction-relu-logits"])
	}
	if got["*"] != "high" {
		t.Fatalf("expected *=high, got %q", got["*"])
	}
}

func TestParseTaskSelectorSet_SupportsCommaAndSpace(t *testing.T) {
	got := parseTaskSelectorSet("'model-extraction-relu-logits, regex-chess *'")
	if _, ok := got["model-extraction-relu-logits"]; !ok {
		t.Fatal("expected model-extraction-relu-logits in selector set")
	}
	if _, ok := got["regex-chess"]; !ok {
		t.Fatal("expected regex-chess in selector set")
	}
	if _, ok := got["*"]; !ok {
		t.Fatal("expected wildcard selector in selector set")
	}
}

func TestShouldDisableReasoningSandwichByTask_TaskAndWildcard(t *testing.T) {
	t.Setenv("GOLLEM_REASONING_NO_SANDWICH_BY_TASK", "'model-extraction-relu-logits,*'")
	t.Setenv("GOLLEM_TASK_NAME", "model-extraction-relu-logits")

	disabled, source := shouldDisableReasoningSandwichByTask("/app")
	if !disabled {
		t.Fatal("expected sandwich disable for exact task")
	}
	if source != "task:model-extraction-relu-logits" {
		t.Fatalf("unexpected source: %q", source)
	}

	t.Setenv("GOLLEM_TASK_NAME", "some-other-task")
	disabled, source = shouldDisableReasoningSandwichByTask("/app")
	if !disabled {
		t.Fatal("expected sandwich disable for wildcard task")
	}
	if source != "task:*" {
		t.Fatalf("unexpected source for wildcard: %q", source)
	}
}

func TestShouldDisableReasoningSandwichByTask_NoMatch(t *testing.T) {
	t.Setenv("GOLLEM_REASONING_NO_SANDWICH_BY_TASK", "model-extraction-relu-logits")
	t.Setenv("GOLLEM_TASK_NAME", "regex-chess")

	disabled, source := shouldDisableReasoningSandwichByTask("/app")
	if disabled {
		t.Fatalf("expected sandwich to remain enabled, source=%q", source)
	}
}

func TestShouldDisableGreedyPressureByTask(t *testing.T) {
	t.Setenv("GOLLEM_REASONING_NO_GREEDY_BY_TASK", "'model-extraction-relu-logits,*'")
	t.Setenv("GOLLEM_TASK_NAME", "model-extraction-relu-logits")

	disabled, source := shouldDisableGreedyPressureByTask("/app")
	if !disabled {
		t.Fatal("expected greedy pressure disable for exact task")
	}
	if source != "task:model-extraction-relu-logits" {
		t.Fatalf("unexpected source: %q", source)
	}

	t.Setenv("GOLLEM_TASK_NAME", "other-task")
	disabled, source = shouldDisableGreedyPressureByTask("/app")
	if !disabled {
		t.Fatal("expected greedy pressure disable for wildcard")
	}
	if source != "task:*" {
		t.Fatalf("unexpected wildcard source: %q", source)
	}
}
