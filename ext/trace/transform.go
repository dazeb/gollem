package trace

import (
	"encoding/json"
	"errors"
	"strings"
)

// RedactOptions controls best-effort local redaction of trace artifacts.
type RedactOptions struct {
	Keys        []string
	Patterns    []string
	Replacement string
	DropTrace   bool
}

// CompactOptions controls trace artifact size reduction.
type CompactOptions struct {
	DropTrace         bool
	EventPayloadLimit int
	KeepSnapshots     int
}

// Redact returns a redacted copy of artifact. It redacts configured key names
// anywhere in the artifact and literal pattern occurrences inside strings.
func Redact(artifact *Artifact, opts RedactOptions) (*Artifact, error) {
	if artifact == nil {
		return nil, errors.New("nil trace artifact")
	}
	replacement := opts.Replacement
	if replacement == "" {
		replacement = "[REDACTED]"
	}
	keys := opts.Keys
	if len(keys) == 0 {
		keys = defaultRedactKeys()
	}

	generic, err := artifactAsGeneric(artifact)
	if err != nil {
		return nil, err
	}
	changed := redactValue(generic, redactConfig{
		keys:        normalizeRedactKeys(keys),
		patterns:    opts.Patterns,
		replacement: replacement,
	})
	redacted, err := artifactFromGeneric(generic)
	if err != nil {
		return nil, err
	}
	if opts.DropTrace {
		redacted.Trace = nil
	}
	if redacted.Metadata == nil {
		redacted.Metadata = make(map[string]any)
	}
	redacted.Metadata["redacted"] = true
	for i := range redacted.Events {
		if changed {
			redacted.Events[i].Redacted = true
			redacted.Events[i].Redaction = &RedactionMetadata{
				Applied:     true,
				Keys:        keys,
				Patterns:    len(opts.Patterns),
				Replacement: replacement,
			}
		}
	}
	return redacted, nil
}

// Compact returns a smaller copy of artifact while preserving summary and
// canonical event structure.
func Compact(artifact *Artifact, opts CompactOptions) (*Artifact, error) {
	if artifact == nil {
		return nil, errors.New("nil trace artifact")
	}
	generic, err := artifactAsGeneric(artifact)
	if err != nil {
		return nil, err
	}
	compacted, err := artifactFromGeneric(generic)
	if err != nil {
		return nil, err
	}
	if opts.DropTrace {
		compacted.Trace = nil
	}
	if opts.KeepSnapshots >= 0 && opts.KeepSnapshots < len(compacted.Snapshots) {
		start := len(compacted.Snapshots) - opts.KeepSnapshots
		compacted.Snapshots = append([]SnapshotRecord(nil), compacted.Snapshots[start:]...)
	}
	if opts.EventPayloadLimit > 0 {
		for i := range compacted.Events {
			compacted.Events[i].Payload = compactPayload(compacted.Events[i].Payload, opts.EventPayloadLimit)
		}
	}
	if compacted.Metadata == nil {
		compacted.Metadata = make(map[string]any)
	}
	compacted.Metadata["compacted"] = true
	return compacted, nil
}

type redactConfig struct {
	keys        map[string]struct{}
	patterns    []string
	replacement string
}

func defaultRedactKeys() []string {
	return []string{
		"api_key",
		"apikey",
		"authorization",
		"cookie",
		"password",
		"secret",
		"token",
	}
}

func normalizeRedactKeys(keys []string) map[string]struct{} {
	out := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		key = strings.ToLower(strings.TrimSpace(key))
		if key != "" {
			out[key] = struct{}{}
		}
	}
	return out
}

func redactValue(value any, cfg redactConfig) bool {
	switch typed := value.(type) {
	case map[string]any:
		changed := false
		for key, child := range typed {
			if shouldRedactKey(key, cfg.keys) {
				typed[key] = cfg.replacement
				changed = true
				continue
			}
			if s, ok := child.(string); ok {
				redacted := redactString(s, cfg.patterns, cfg.replacement)
				if redacted != s {
					typed[key] = redacted
					changed = true
				}
				continue
			}
			if redactValue(child, cfg) {
				changed = true
			}
		}
		return changed
	case []any:
		changed := false
		for i, child := range typed {
			if s, ok := child.(string); ok {
				redacted := redactString(s, cfg.patterns, cfg.replacement)
				if redacted != s {
					typed[i] = redacted
					changed = true
				}
				continue
			}
			if redactValue(child, cfg) {
				typed[i] = child
				changed = true
			}
		}
		return changed
	case string:
		redacted := redactString(typed, cfg.patterns, cfg.replacement)
		return redacted != typed
	default:
		return false
	}
}

func shouldRedactKey(key string, keys map[string]struct{}) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	if _, ok := keys[key]; ok {
		return true
	}
	for sensitive := range keys {
		if sensitive == "token" && strings.Contains(key, "tokens") {
			continue
		}
		if sensitive != "" && strings.Contains(key, sensitive) {
			return true
		}
	}
	return false
}

func redactString(value string, patterns []string, replacement string) string {
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		value = strings.ReplaceAll(value, pattern, replacement)
	}
	return value
}

func compactPayload(payload map[string]any, maxString int) map[string]any {
	if len(payload) == 0 {
		return nil
	}
	data, err := json.Marshal(payload)
	if err != nil || len(data) <= maxString {
		return payload
	}
	return map[string]any{
		"compacted": true,
		"bytes":     len(data),
		"preview":   truncateLine(string(data), maxString),
	}
}

func artifactAsGeneric(artifact *Artifact) (map[string]any, error) {
	data, err := json.Marshal(artifact)
	if err != nil {
		return nil, err
	}
	var generic map[string]any
	if err := json.Unmarshal(data, &generic); err != nil {
		return nil, err
	}
	return generic, nil
}

func artifactFromGeneric(generic map[string]any) (*Artifact, error) {
	data, err := json.Marshal(generic)
	if err != nil {
		return nil, err
	}
	var artifact Artifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		return nil, err
	}
	return &artifact, nil
}
