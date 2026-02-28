package modelutil

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/fugue-labs/gollem/core"
)

// PersonalityRequest describes what personality to generate for an agent.
type PersonalityRequest struct {
	// Task is the task being assigned to the agent.
	Task string

	// Role is an optional role hint (e.g., "code reviewer", "test writer").
	Role string

	// BasePrompt is an optional base prompt to extend. When provided,
	// the generated personality incorporates these instructions.
	BasePrompt string

	// Constraints are optional behavioral constraints (e.g., "never modify tests").
	Constraints []string

	// Context provides optional extra context as key-value pairs.
	Context map[string]string
}

// PersonalityGeneratorFunc generates a task-specific system prompt from a request.
type PersonalityGeneratorFunc func(ctx context.Context, req PersonalityRequest) (string, error)

// GeneratePersonality returns a PersonalityGeneratorFunc that uses the given model
// to generate a task-specific system prompt via a meta-prompt.
func GeneratePersonality(model core.Model) PersonalityGeneratorFunc {
	return func(ctx context.Context, req PersonalityRequest) (string, error) {
		if req.Task == "" {
			return "", errors.New("personality request requires a task")
		}

		prompt := buildMetaPrompt(req)

		messages := []core.ModelMessage{
			core.ModelRequest{
				Parts: []core.ModelRequestPart{
					core.SystemPromptPart{Content: personalityMetaSystem},
					core.UserPromptPart{Content: prompt},
				},
			},
		}

		sessionModel := NewSessionModel(model)
		resp, err := sessionModel.Request(ctx, messages, &core.ModelSettings{
			MaxTokens: core.IntPtr(1024),
		}, nil)
		if err != nil {
			return "", fmt.Errorf("personality generation failed: %w", err)
		}

		generated := strings.TrimSpace(resp.TextContent())
		if generated == "" {
			return "", errors.New("personality generation returned empty result")
		}

		return generated, nil
	}
}

// CachedPersonalityGenerator wraps a generator with an in-memory cache.
// Identical PersonalityRequests (by SHA256 of JSON serialization) return
// cached results without making additional model calls.
func CachedPersonalityGenerator(gen PersonalityGeneratorFunc) PersonalityGeneratorFunc {
	var cache sync.Map
	return func(ctx context.Context, req PersonalityRequest) (string, error) {
		key := personalityCacheKey(req)
		if val, ok := cache.Load(key); ok {
			return val.(string), nil
		}
		result, err := gen(ctx, req)
		if err != nil {
			return "", err
		}
		cache.Store(key, result)
		return result, nil
	}
}

func personalityCacheKey(req PersonalityRequest) string {
	data, _ := json.Marshal(req)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func buildMetaPrompt(req PersonalityRequest) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Generate a system prompt for an agent with the following task:\n\n")
	fmt.Fprintf(&b, "TASK: %s\n", req.Task)

	if req.Role != "" {
		fmt.Fprintf(&b, "\nROLE: %s\n", req.Role)
	}

	if req.BasePrompt != "" {
		fmt.Fprintf(&b, "\nBASE INSTRUCTIONS (incorporate and extend these):\n%s\n", req.BasePrompt)
	}

	if len(req.Constraints) > 0 {
		fmt.Fprintf(&b, "\nCONSTRAINTS:\n")
		for _, c := range req.Constraints {
			fmt.Fprintf(&b, "- %s\n", c)
		}
	}

	if len(req.Context) > 0 {
		fmt.Fprintf(&b, "\nADDITIONAL CONTEXT:\n")
		for k, v := range req.Context {
			fmt.Fprintf(&b, "- %s: %s\n", k, v)
		}
	}

	return b.String()
}

const personalityMetaSystem = `You are a prompt engineer. Your job is to write system prompts for AI agents that solve tasks in a terminal environment.

Given a task description, role, and optional constraints, generate a focused system prompt that:
1. Identifies the domain(s) the task belongs to (e.g., spectroscopy, NLP, networking, compilers, signal processing, etc.) and frames the agent as a subject-matter expert in that domain
2. Includes specific domain knowledge, terminology, and techniques relevant to the task — the kind of knowledge a human expert would bring
3. Sets clear behavioral guidelines tailored to the work
4. Incorporates any base instructions provided
5. Respects all stated constraints

Rules for the output:
- Output ONLY the system prompt text, nothing else
- No markdown wrapping, no explanations, no preamble
- Keep it concise (under 500 words) but actionable
- Be specific to the task domain, not generic
- Lead with domain expertise ("You are an expert in X with deep knowledge of Y") rather than generic coding ability
- Include concrete technical details: algorithms, formulas, parameter ranges, common pitfalls, and best practices specific to the domain
- The agent already knows how to code — focus on the specialized knowledge it needs to solve this particular problem correctly`
