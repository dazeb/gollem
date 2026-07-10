package modelutil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/fugue-labs/gollem/core"
)

// StableCacheKeyInput is the provider-neutral material used to key a cached
// model response.
type StableCacheKeyInput struct {
	Provider string
	Model    string
	Messages []core.ModelMessage
	Settings *core.ModelSettings
	Params   *core.ModelRequestParameters
}

// StableCacheKey hashes a model request after stripping request metadata that
// is unstable but not semantically relevant to the prompt.
func StableCacheKey(input StableCacheKeyInput) (string, error) {
	messages, err := core.EncodeMessages(input.Messages)
	if err != nil {
		return "", fmt.Errorf("stable cache key: encoding messages: %w", err)
	}
	payload := map[string]any{
		"messages": messages,
	}
	if input.Settings != nil {
		payload["settings"] = input.Settings
	}
	if input.Params != nil {
		payload["params"] = input.Params
	}
	return StableCacheKeyFromJSON(input.Provider, input.Model, payload)
}

// StableCacheKeyFromJSON hashes provider-shaped request payloads with the same
// normalizer used by StableCacheKey. It is intended for provider wrappers,
// fixture benchmarks, and tests that need to validate transport-only fields.
func StableCacheKeyFromJSON(provider, model string, payload any) (string, error) {
	material := map[string]any{
		"request": payload,
	}
	if provider != "" {
		material["provider"] = provider
	}
	if model != "" {
		material["model"] = model
	}
	return stableHash(material)
}

// StableCacheKeyPayload returns the canonical JSON bytes that StableCacheKey
// hashes. This is useful for diagnostics and benchmark miss explanations.
func StableCacheKeyPayload(input StableCacheKeyInput) ([]byte, error) {
	messages, err := core.EncodeMessages(input.Messages)
	if err != nil {
		return nil, fmt.Errorf("stable cache key payload: encoding messages: %w", err)
	}
	payload := map[string]any{"messages": messages}
	if input.Settings != nil {
		payload["settings"] = input.Settings
	}
	if input.Params != nil {
		payload["params"] = input.Params
	}
	material := map[string]any{"request": payload}
	if input.Provider != "" {
		material["provider"] = input.Provider
	}
	if input.Model != "" {
		material["model"] = input.Model
	}
	return stablePayload(material)
}

func stableHash(material any) (string, error) {
	payload, err := stablePayload(material)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func stablePayload(material any) ([]byte, error) {
	raw, err := json.Marshal(material)
	if err != nil {
		return nil, fmt.Errorf("stable cache key: marshaling material: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var decoded any
	if err := dec.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("stable cache key: decoding material: %w", err)
	}
	normalized, err := normalizeCacheValue(decoded, nil)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("stable cache key: marshaling normalized material: %w", err)
	}
	return payload, nil
}

func normalizeCacheValue(value any, path []string) (any, error) {
	switch v := value.(type) {
	case map[string]any:
		return normalizeCacheMap(v, path)
	case []any:
		return normalizeCacheArray(v, path)
	case string:
		return v, nil
	default:
		return v, nil
	}
}

func normalizeCacheMap(value map[string]any, path []string) (map[string]any, error) {
	out := make(map[string]any, len(value))
	inSchemaProperties := len(path) > 0 && isSchemaPropertyContainer(path[len(path)-1])
	for key, raw := range value {
		if !inSchemaProperties && shouldDropCacheKey(key, value) {
			continue
		}
		normalized, err := normalizeCacheField(key, raw, append(path, key))
		if err != nil {
			return nil, err
		}
		out[key] = normalized
	}
	return out, nil
}

func normalizeCacheField(key string, raw any, path []string) (any, error) {
	if s, ok := raw.(string); ok && shouldCanonicalizeJSONString(key) {
		canonical, err := canonicalizeJSONString(s, path)
		if err != nil {
			return nil, err
		}
		return canonical, nil
	}
	return normalizeCacheValue(raw, path)
}

func normalizeCacheArray(value []any, path []string) ([]any, error) {
	out := make([]any, 0, len(value))
	for _, item := range value {
		normalized, err := normalizeCacheValue(item, path)
		if err != nil {
			return nil, err
		}
		out = append(out, normalized)
	}
	if len(path) > 0 && shouldSortCacheArray(path[len(path)-1], out) {
		sort.SliceStable(out, func(i, j int) bool {
			return canonicalSortString(out[i]) < canonicalSortString(out[j])
		})
	}
	return out, nil
}

func shouldDropCacheKey(key string, parent map[string]any) bool {
	switch normalizedKey(key) {
	case "timestamp",
		"created",
		"createdat",
		"updatedat",
		"startedat",
		"endedat",
		"duration",
		"requestid",
		"traceid",
		"runid",
		"parentrunid",
		"turnid",
		"randomseed",
		"seed",
		"promptcachekey",
		"promptcacheretention",
		"cachecontrol",
		"servicetier",
		"stream",
		"streamoptions",
		"previousresponseid",
		"signature",
		"usage",
		"modelname",
		"finishreason",
		"toolcallid",
		"tooluseid":
		return true
	case "id":
		return parentHasGeneratedProviderID(parent)
	default:
		return false
	}
}

func parentHasGeneratedProviderID(parent map[string]any) bool {
	if typeValue, ok := parent["type"].(string); ok {
		switch typeValue {
		case "tool_use", "function":
			return true
		}
	}
	if _, ok := parent["function"]; ok {
		return true
	}
	return false
}

func shouldCanonicalizeJSONString(key string) bool {
	switch normalizedKey(key) {
	case "argsjson", "arguments", "input":
		return true
	default:
		return false
	}
}

func canonicalizeJSONString(value string, path []string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return value, nil
	}
	decoded, ok := decodeJSONString(value)
	if !ok {
		return value, nil
	}
	normalized, err := normalizeCacheValue(decoded, path)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("stable cache key: canonicalizing JSON string: %w", err)
	}
	return string(encoded), nil
}

func decodeJSONString(value string) (any, bool) {
	dec := json.NewDecoder(strings.NewReader(value))
	dec.UseNumber()
	var decoded any
	if dec.Decode(&decoded) != nil {
		return nil, false
	}
	if hasTrailingJSON(dec) {
		return nil, false
	}
	return decoded, true
}

func hasTrailingJSON(dec *json.Decoder) bool {
	var trailing any
	return dec.Decode(&trailing) != io.EOF
}

func shouldSortCacheArray(key string, values []any) bool {
	if len(values) < 2 {
		return false
	}
	switch normalizedKey(key) {
	case "required", "enum":
		return allScalarCacheValues(values)
	case "functiontools", "outputtools", "tools":
		return allToolLikeValues(values)
	default:
		return false
	}
}

func allScalarCacheValues(values []any) bool {
	for _, value := range values {
		switch value.(type) {
		case string, bool, nil, json.Number:
		default:
			return false
		}
	}
	return true
}

func allToolLikeValues(values []any) bool {
	for _, value := range values {
		m, ok := value.(map[string]any)
		if !ok {
			return false
		}
		if _, ok := m["name"]; ok {
			continue
		}
		if fn, ok := m["function"].(map[string]any); ok {
			if _, ok := fn["name"]; ok {
				continue
			}
		}
		if _, ok := m["Name"]; ok {
			continue
		}
		return false
	}
	return true
}

func canonicalSortString(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(encoded)
}

func isSchemaPropertyContainer(key string) bool {
	switch normalizedKey(key) {
	case "properties", "defs", "definitions", "$defs":
		return true
	default:
		return false
	}
}

func normalizedKey(key string) string {
	replacer := strings.NewReplacer("_", "", "-", "", ".", "")
	return strings.ToLower(replacer.Replace(key))
}
