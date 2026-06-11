package core

// WithAdaptiveThinking lets the model decide when and how much to think
// before answering (Anthropic adaptive thinking, Claude 4.6 generation
// and newer; the only thinking mode on Opus 4.7+). Mutually exclusive
// with WithThinkingBudget — the Anthropic providers reject requests that
// set both. Thinking tokens count toward MaxTokens, so pair this with a
// generous WithMaxTokens. See ModelSettings.AdaptiveThinking.
func WithAdaptiveThinking[T any](on bool) AgentOption[T] {
	return func(a *Agent[T]) {
		if a.modelSettings == nil {
			a.modelSettings = &ModelSettings{}
		}
		a.modelSettings.AdaptiveThinking = &on
	}
}
