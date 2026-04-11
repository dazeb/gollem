package toolproxy

import (
	"fmt"
	"sort"
	"strings"

	"github.com/fugue-labs/gollem/core"
)

const toolSearchDescription = `Loads deferred tool definitions on demand.

The agent has access to additional tools that are not listed in the
default tools array to save tokens. Their names are visible in the
system prompt under "Deferred Tools". Call this tool to load the full
schemas for specific deferred tools so you can then invoke them.

Query forms:
  - "select:NameA,NameB,NameC" — load these exact tools by name.
  - "github issue create" — keyword search over deferred tool names,
    descriptions, and search hints (up to max_results best matches).
  - "+slack send" — require "slack" in the name or description, then
    rank by remaining terms.

Result: the matched tool names become available to call on subsequent
turns. Their full schemas are included in the next request's tool list.`

// buildSystemPromptFragment returns a system-prompt section listing the
// names of all deferred tools in ` + "`" + `tools` + "`" + ` so the model knows what it can
// ask tool_search to load. If no tools are deferred the function returns
// an empty string, which is safe to concatenate.
//
// The output intentionally mirrors Claude Code's <available-deferred-tools>
// block: one tool per line, name only, no descriptions (empirically the
// description variant showed no scoring benefit and doubled token cost).
func buildSystemPromptFragment(toolName string, tools []core.Tool) string {
	var deferredNames []string
	for _, t := range tools {
		if t.ShouldDefer {
			deferredNames = append(deferredNames, t.Definition.Name)
		}
	}
	if len(deferredNames) == 0 {
		return ""
	}
	sort.Strings(deferredNames)

	var b strings.Builder
	b.WriteString("\n\n## Deferred Tools\n\n")
	fmt.Fprintf(&b, "These tools are available but not loaded into the default tool list to save tokens. "+
		"Call `%s` to load the full schema for any of them before invoking:\n\n", toolName)
	for _, n := range deferredNames {
		fmt.Fprintf(&b, "  - %s\n", n)
	}
	b.WriteString("\nOnce loaded, a deferred tool stays available for the rest of this run.\n")
	return b.String()
}

// buildToolResultText produces the human-readable text returned by
// tool_search when one or more tools are resolved. The body lists the
// matched tools with their (possibly truncated) descriptions so the
// model can plan its next turn; the full schema is delivered through
// the next request's tool definitions (PrepareFunc re-admits them to
// the list).
//
// Wording is neutral ("Matched") rather than "Loaded" because a match
// may hit a tool that was already non-deferred and therefore already
// available. Either way, the accurate summary is "you can call these
// on your next turn".
func buildToolResultText(matches []string, tools []core.Tool) string {
	if len(matches) == 0 {
		return "No matching tools found."
	}

	byName := make(map[string]core.Tool, len(tools))
	for _, t := range tools {
		byName[t.Definition.Name] = t
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Matched %d tool(s). They are available to call on your next turn:\n\n", len(matches))
	for _, name := range matches {
		t, ok := byName[name]
		if !ok {
			fmt.Fprintf(&b, "  - %s\n", name)
			continue
		}
		desc := firstLine(t.Definition.Description)
		if desc == "" {
			fmt.Fprintf(&b, "  - %s\n", name)
		} else {
			fmt.Fprintf(&b, "  - %s — %s\n", name, desc)
		}
	}
	return b.String()
}

// firstLine returns the first line of s with surrounding whitespace
// stripped. Used to keep tool_search results compact — long descriptions
// would defeat the point of deferral.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}
