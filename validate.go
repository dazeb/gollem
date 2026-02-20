package gollem

import (
	"context"
	"encoding/json"
	"fmt"
)

// OutputValidatorFunc validates and potentially transforms the output.
// Return a ModelRetryError to trigger a retry with feedback to the model.
type OutputValidatorFunc[T any] func(ctx context.Context, rc *RunContext, output T) (T, error)

// validateOutput runs all output validators in sequence.
func validateOutput[T any](ctx context.Context, rc *RunContext, output T, validators []OutputValidatorFunc[T]) (T, error) {
	for _, v := range validators {
		var err error
		output, err = v(ctx, rc, output)
		if err != nil {
			return output, err
		}
	}
	return output, nil
}

// deserializeOutput deserializes JSON into type T, handling the outer_typed_dict_key
// unwrapping pattern. When outerKey is set, the JSON is expected to be an object
// with a single key matching outerKey, whose value is the actual output.
func deserializeOutput[T any](argsJSON string, outerKey string) (T, error) {
	var zero T

	data := []byte(argsJSON)

	// Handle outer key unwrapping.
	if outerKey != "" {
		var wrapper map[string]json.RawMessage
		if err := json.Unmarshal(data, &wrapper); err != nil {
			return zero, fmt.Errorf("failed to unmarshal output wrapper: %w", err)
		}
		inner, ok := wrapper[outerKey]
		if !ok {
			return zero, fmt.Errorf("output wrapper missing key %q", outerKey)
		}
		data = inner
	}

	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		return zero, fmt.Errorf("failed to unmarshal output: %w", err)
	}
	return result, nil
}
