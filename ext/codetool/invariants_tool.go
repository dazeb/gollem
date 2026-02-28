package codetool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/fugue-labs/gollem/core"
)

type invariantItem struct {
	ID          string `json:"id" jsonschema:"description=Stable identifier (e.g., I1, I2)"`
	Description string `json:"description" jsonschema:"description=Constraint text that must be satisfied"`
	Kind        string `json:"kind,omitempty" jsonschema:"description=hard or soft"`
	Status      string `json:"status,omitempty" jsonschema:"description=unknown, in_progress, pass, or fail"`
	Evidence    string `json:"evidence,omitempty" jsonschema:"description=Concrete command/file evidence for status"`
}

type invariantCommand struct {
	Command     string          `json:"command" jsonschema:"description=extract|get|summary|update|add"`
	ID          string          `json:"id,omitempty" jsonschema:"description=Invariant ID for update command"`
	Status      string          `json:"status,omitempty" jsonschema:"description=unknown|in_progress|pass|fail (for update command)"`
	Evidence    string          `json:"evidence,omitempty" jsonschema:"description=Evidence note for update command"`
	Description string          `json:"description,omitempty" jsonschema:"description=Invariant description for add command"`
	Kind        string          `json:"kind,omitempty" jsonschema:"description=hard|soft for add command"`
	Items       []invariantItem `json:"items,omitempty" jsonschema:"description=Optional batch items for add command"`
}

type invariantExtractResponse struct {
	Items []struct {
		ID          string `json:"id"`
		Description string `json:"description"`
		Kind        string `json:"kind"`
	} `json:"items"`
}

type invariantState struct {
	mu        sync.Mutex
	items     []invariantItem
	extracted bool
}

func (s *invariantState) ExportState() (any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]invariantItem, len(s.items))
	copy(cp, s.items)
	return map[string]any{
		"items":     cp,
		"extracted": s.extracted,
	}, nil
}

func (s *invariantState) RestoreState(state any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, ok := state.(map[string]any)
	if !ok {
		return errors.New("invalid invariants state")
	}

	if v, ok := m["extracted"].(bool); ok {
		s.extracted = v
	}
	if raw, ok := m["items"]; ok {
		b, err := json.Marshal(raw)
		if err != nil {
			return fmt.Errorf("marshal invariants state items: %w", err)
		}
		var items []invariantItem
		if err := json.Unmarshal(b, &items); err != nil {
			return fmt.Errorf("unmarshal invariants state items: %w", err)
		}
		s.items = items
	}
	return nil
}

// InvariantsTool manages a structured invariant checklist.
//
// The checklist is extracted using a dedicated model query (LLM-only), then
// updated by the agent with PASS/FAIL evidence during verification.
// No regex/heuristic fallback is used for extraction.
func InvariantsTool(model core.Model) core.Tool {
	state := &invariantState{}

	tool := core.FuncTool[invariantCommand](
		"invariants",
		"Manage a structured invariant checklist for the current task. "+
			"Commands: "+
			"'extract' (LLM-extract hard/soft constraints from the task prompt), "+
			"'get' (return all invariants), "+
			"'summary' (return pass/fail counts, especially hard constraints), "+
			"'update' (set one invariant status with evidence), "+
			"'add' (add one or many invariants). "+
			"Use this before final completion to prove every HARD invariant is PASS.",
		func(ctx context.Context, rc *core.RunContext, cmd invariantCommand) (any, error) {
			switch strings.ToLower(strings.TrimSpace(cmd.Command)) {
			case "extract":
				prompt := ""
				if rc != nil {
					prompt = strings.TrimSpace(rc.Prompt)
				}
				if prompt == "" {
					return nil, errors.New("extract requires non-empty run prompt")
				}
				items, err := extractInvariantsWithModel(ctx, model, prompt)
				if err != nil {
					return nil, err
				}
				state.mu.Lock()
				state.items = normalizeInvariantItems(items)
				state.extracted = true
				resp := invariantsSummaryLocked(state.items, state.extracted)
				state.mu.Unlock()
				return resp, nil

			case "get":
				state.mu.Lock()
				resp := invariantsSummaryLocked(state.items, state.extracted)
				state.mu.Unlock()
				return resp, nil

			case "summary":
				state.mu.Lock()
				resp := invariantsSummaryLocked(state.items, state.extracted)
				state.mu.Unlock()
				return resp, nil

			case "update":
				if strings.TrimSpace(cmd.ID) == "" {
					return nil, errors.New("update requires id")
				}
				status := normalizeInvariantStatus(cmd.Status)
				if status == "" {
					return nil, errors.New("update requires status in {unknown,in_progress,pass,fail}")
				}
				state.mu.Lock()
				defer state.mu.Unlock()
				for i := range state.items {
					if strings.EqualFold(state.items[i].ID, cmd.ID) {
						state.items[i].Status = status
						if strings.TrimSpace(cmd.Evidence) != "" {
							state.items[i].Evidence = strings.TrimSpace(cmd.Evidence)
						}
						return invariantsSummaryLocked(state.items, state.extracted), nil
					}
				}
				return nil, fmt.Errorf("invariant id %q not found", cmd.ID)

			case "add":
				state.mu.Lock()
				defer state.mu.Unlock()
				if strings.TrimSpace(cmd.Description) != "" {
					nextID := nextInvariantIDLocked(state.items)
					state.items = append(state.items, invariantItem{
						ID:          nextID,
						Description: strings.TrimSpace(cmd.Description),
						Kind:        normalizeInvariantKind(cmd.Kind),
						Status:      "unknown",
					})
					return invariantsSummaryLocked(state.items, state.extracted), nil
				}
				if len(cmd.Items) == 0 {
					return nil, errors.New("add requires description or items")
				}
				for _, it := range cmd.Items {
					desc := strings.TrimSpace(it.Description)
					if desc == "" {
						continue
					}
					id := strings.TrimSpace(it.ID)
					if id == "" {
						id = nextInvariantIDLocked(state.items)
					}
					state.items = append(state.items, invariantItem{
						ID:          id,
						Description: desc,
						Kind:        normalizeInvariantKind(it.Kind),
						Status:      normalizeInvariantStatusOrDefault(it.Status),
						Evidence:    strings.TrimSpace(it.Evidence),
					})
				}
				state.items = dedupeInvariantItems(state.items)
				return invariantsSummaryLocked(state.items, state.extracted), nil

			default:
				return nil, fmt.Errorf("unknown command %q (use extract|get|summary|update|add)", cmd.Command)
			}
		},
	)
	tool.Stateful = state
	return tool
}

func extractInvariantsWithModel(ctx context.Context, model core.Model, taskPrompt string) ([]invariantItem, error) {
	maxTokens := 2000
	temp := 0.0
	msgs := []core.ModelMessage{
		core.ModelRequest{
			Parts: []core.ModelRequestPart{
				core.SystemPromptPart{Content: "Extract task constraints as strict JSON. " +
					"Return ONLY JSON with shape: {\"items\":[{\"id\":\"I1\",\"description\":\"...\",\"kind\":\"hard|soft\"}]}. " +
					"Hard constraints are objective, verifier-checkable requirements (required files, exact formats, explicit thresholds, forbidden actions). " +
					"Soft constraints are preferences or weaker guidance. Include at most 12 items."},
				core.UserPromptPart{Content: taskPrompt},
			},
		},
	}
	resp, err := model.Request(ctx, msgs, &core.ModelSettings{
		MaxTokens:   &maxTokens,
		Temperature: &temp,
	}, &core.ModelRequestParameters{AllowTextOutput: true})
	if err != nil {
		return nil, fmt.Errorf("invariants extract model call failed: %w", err)
	}

	parsed, err := parseInvariantExtractResponse(resp.TextContent())
	if err != nil {
		return nil, err
	}
	items := make([]invariantItem, 0, len(parsed.Items))
	for _, it := range parsed.Items {
		desc := strings.TrimSpace(it.Description)
		if desc == "" {
			continue
		}
		items = append(items, invariantItem{
			ID:          strings.TrimSpace(it.ID),
			Description: desc,
			Kind:        normalizeInvariantKind(it.Kind),
			Status:      "unknown",
		})
	}
	if len(items) == 0 {
		return nil, errors.New("invariants extract returned no valid items")
	}
	return items, nil
}

func parseInvariantExtractResponse(text string) (*invariantExtractResponse, error) {
	raw := strings.TrimSpace(text)
	if raw == "" {
		return nil, errors.New("invariants extract returned empty response")
	}

	// Accept fenced JSON but do not use any heuristic extraction fallback.
	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```json")
		raw = strings.TrimPrefix(raw, "```JSON")
		raw = strings.TrimPrefix(raw, "```")
		raw = strings.TrimSuffix(raw, "```")
		raw = strings.TrimSpace(raw)
	}

	var out invariantExtractResponse
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("invariants extract response must be valid JSON: %w", err)
	}
	return &out, nil
}

func invariantsSummaryLocked(items []invariantItem, extracted bool) map[string]any {
	cp := make([]invariantItem, len(items))
	copy(cp, items)
	sort.Slice(cp, func(i, j int) bool {
		return strings.ToLower(cp[i].ID) < strings.ToLower(cp[j].ID)
	})

	hardTotal, hardPass, hardFail := 0, 0, 0
	softTotal, softPass, softFail := 0, 0, 0
	for _, it := range cp {
		if normalizeInvariantKind(it.Kind) == "hard" {
			hardTotal++
			switch normalizeInvariantStatusOrDefault(it.Status) {
			case "pass":
				hardPass++
			case "fail":
				hardFail++
			}
		} else {
			softTotal++
			switch normalizeInvariantStatusOrDefault(it.Status) {
			case "pass":
				softPass++
			case "fail":
				softFail++
			}
		}
	}
	return map[string]any{
		"status":          "ok",
		"extracted":       extracted,
		"items":           cp,
		"hard_total":      hardTotal,
		"hard_pass":       hardPass,
		"hard_fail":       hardFail,
		"hard_unresolved": hardTotal - hardPass - hardFail,
		"soft_total":      softTotal,
		"soft_pass":       softPass,
		"soft_fail":       softFail,
	}
}

func normalizeInvariantItems(items []invariantItem) []invariantItem {
	out := make([]invariantItem, 0, len(items))
	for i, it := range items {
		desc := strings.TrimSpace(it.Description)
		if desc == "" {
			continue
		}
		// Canonicalize all IDs to I-prefix so nextInvariantIDLocked can
		// always find the max. Models sometimes return C1, H2, etc.
		id := fmt.Sprintf("I%d", i+1)
		out = append(out, invariantItem{
			ID:          id,
			Description: desc,
			Kind:        normalizeInvariantKind(it.Kind),
			Status:      normalizeInvariantStatusOrDefault(it.Status),
			Evidence:    strings.TrimSpace(it.Evidence),
		})
	}
	out = dedupeInvariantItems(out)
	if len(out) > 12 {
		out = out[:12]
	}
	return out
}

func dedupeInvariantItems(items []invariantItem) []invariantItem {
	seenDesc := make(map[string]bool)
	out := make([]invariantItem, 0, len(items))
	for _, it := range items {
		key := strings.ToLower(strings.TrimSpace(it.Description))
		if key == "" || seenDesc[key] {
			continue
		}
		seenDesc[key] = true
		out = append(out, it)
	}
	return out
}

func nextInvariantIDLocked(items []invariantItem) string {
	maxN := 0
	for _, it := range items {
		id := strings.TrimSpace(strings.ToUpper(it.ID))
		if len(id) < 2 || id[0] != 'I' {
			continue
		}
		var n int
		if _, err := fmt.Sscanf(id, "I%d", &n); err == nil && n > maxN {
			maxN = n
		}
	}
	return fmt.Sprintf("I%d", maxN+1)
}

func normalizeInvariantKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "hard":
		return "hard"
	case "soft":
		return "soft"
	default:
		return "hard"
	}
}

func normalizeInvariantStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "unknown":
		return "unknown"
	case "in_progress", "in-progress", "wip":
		return "in_progress"
	case "pass", "passed":
		return "pass"
	case "fail", "failed":
		return "fail"
	default:
		return ""
	}
}

func normalizeInvariantStatusOrDefault(status string) string {
	if s := normalizeInvariantStatus(status); s != "" {
		return s
	}
	return "unknown"
}
