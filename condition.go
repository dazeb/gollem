package gollem

import "context"

// RunCondition is a predicate checked after each model response.
// Return true to stop the run, with an optional reason message.
type RunCondition func(ctx context.Context, rc *RunContext, resp *ModelResponse) (stop bool, reason string)
