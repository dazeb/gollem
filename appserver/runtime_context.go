package appserver

import "context"

type runtimeTurnContextKey struct{}

type runtimeTurnContext struct {
	ThreadID string
	TurnID   string
}

func withRuntimeTurnContext(ctx context.Context, threadID, turnID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, runtimeTurnContextKey{}, runtimeTurnContext{
		ThreadID: threadID,
		TurnID:   turnID,
	})
}

func runtimeTurnContextFrom(ctx context.Context) runtimeTurnContext {
	if ctx == nil {
		return runtimeTurnContext{}
	}
	value, _ := ctx.Value(runtimeTurnContextKey{}).(runtimeTurnContext)
	return value
}
