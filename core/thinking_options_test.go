package core

import "testing"

func TestWithAdaptiveThinking(t *testing.T) {
	a := &Agent[string]{}
	WithAdaptiveThinking[string](true)(a)
	if a.modelSettings == nil || a.modelSettings.AdaptiveThinking == nil || !*a.modelSettings.AdaptiveThinking {
		t.Fatalf("expected AdaptiveThinking=true on settings, got %+v", a.modelSettings)
	}

	a = &Agent[string]{}
	WithAdaptiveThinking[string](false)(a)
	if a.modelSettings == nil || a.modelSettings.AdaptiveThinking == nil || *a.modelSettings.AdaptiveThinking {
		t.Fatalf("expected explicit AdaptiveThinking=false on settings, got %+v", a.modelSettings)
	}
}
