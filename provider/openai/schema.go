package openai

import (
	"encoding/json"

	"github.com/fugue-labs/gollem/core"
)

// marshalOpenAISchema normalizes JSON schema to satisfy OpenAI validators.
// OpenAI requires object schemas to include a "properties" object.
func marshalOpenAISchema(schema core.Schema) (json.RawMessage, error) {
	normalized := normalizeOpenAISchemaAny(schema)
	data, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func normalizeOpenAISchemaAny(v any) any {
	switch s := v.(type) {
	case map[string]any:
		return normalizeOpenAISchemaMap(s)
	case []any:
		out := make([]any, len(s))
		for i := range s {
			out[i] = normalizeOpenAISchemaAny(s[i])
		}
		return out
	default:
		return v
	}
}

func normalizeOpenAISchemaMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in)+1)
	for k, v := range in {
		out[k] = normalizeOpenAISchemaAny(v)
	}

	if typ, ok := out["type"].(string); ok && typ == "object" {
		if _, exists := out["properties"]; !exists {
			out["properties"] = map[string]any{}
		}
	}

	return out
}
