package toolproxy

import (
	"regexp"
	"sort"
	"strings"

	"github.com/fugue-labs/gollem/core"
)

// parsedName is a tool name broken down for keyword matching.
// Ported from Claude Code's parseToolName in ToolSearchTool.ts.
type parsedName struct {
	parts []string // lowercase name fragments (CamelCase split, snake-splits, etc.)
	full  string   // parts joined by spaces
	isMcp bool     // true for names prefixed "mcp__"
}

// parseToolName tokenizes a tool name into searchable parts.
// Handles MCP tools (mcp__server__action) and regular tools (CamelCase
// or snake_case). The rules match Claude Code's implementation so that
// scoring behaves identically for shared models.
func parseToolName(name string) parsedName {
	if strings.HasPrefix(name, "mcp__") {
		withoutPrefix := strings.ToLower(strings.TrimPrefix(name, "mcp__"))
		var parts []string
		for _, seg := range strings.Split(withoutPrefix, "__") {
			for _, sub := range strings.Split(seg, "_") {
				if sub != "" {
					parts = append(parts, sub)
				}
			}
		}
		full := strings.ReplaceAll(strings.ReplaceAll(withoutPrefix, "__", " "), "_", " ")
		return parsedName{parts: parts, full: full, isMcp: true}
	}

	// CamelCase → spaces, then _ → space, then collapse whitespace.
	withSpaces := camelCaseSplitter.ReplaceAllString(name, "$1 $2")
	withSpaces = strings.ReplaceAll(withSpaces, "_", " ")
	withSpaces = strings.ToLower(withSpaces)

	var parts []string
	for _, field := range strings.Fields(withSpaces) {
		parts = append(parts, field)
	}
	return parsedName{
		parts: parts,
		full:  strings.Join(parts, " "),
		isMcp: false,
	}
}

// camelCaseSplitter inserts a delimiter between a lowercase letter followed
// by an uppercase letter so "ToolSearchTool" → "Tool Search Tool".
var camelCaseSplitter = regexp.MustCompile(`([a-z])([A-Z])`)

// compileTermPatterns pre-compiles word-boundary regexes for each distinct
// query term so we only pay the compilation cost once per search rather
// than per-candidate.
func compileTermPatterns(terms []string) map[string]*regexp.Regexp {
	out := make(map[string]*regexp.Regexp, len(terms))
	for _, term := range terms {
		if _, ok := out[term]; ok {
			continue
		}
		// Escape regex metacharacters in the term itself.
		out[term] = regexp.MustCompile(`\b` + regexp.QuoteMeta(term) + `\b`)
	}
	return out
}

// searchResult is one candidate ranked during keyword search.
type searchResult struct {
	name  string
	score int
}

// scoringLookup returns the cached scoringEntry for a tool. The Proxy
// supplies a closure over its per-run cache so searchTools doesn't have
// to know about state.go internals.
type scoringLookup func(core.Tool) scoringEntry

// uncachedLookup parses the tool on every call. Used by tests and by
// the non-state-backed code paths where we don't have a Proxy on hand.
func uncachedLookup(tool core.Tool) scoringEntry {
	return scoringEntry{
		parsed:    parseToolName(tool.Definition.Name),
		descLower: strings.ToLower(tool.Definition.Description),
		hintLower: strings.ToLower(tool.SearchHint),
	}
}

// searchTools runs the keyword-based search. The `pool` argument is the
// full tool list (deferred + non-deferred) that the Proxy recorded on
// the most recent PrepareFunc call. Scoring applies only to deferred
// tools (the point of the feature) but exact-match / select: lookups
// fall back to the full list, so the model can ask for a tool it
// already has and get a harmless no-op instead of "no matches".
//
// `lookup` returns a cached per-tool scoringEntry to avoid recomputing
// parseToolName / ToLower on every search. Pass uncachedLookup when
// calling from a context without a shared cache (e.g. unit tests).
//
// Supported query forms:
//   - `select:Name1,Name2` → bypass scoring, return the exact names that exist
//     (deferred OR non-deferred — matches Claude Code's fallback behavior)
//   - `mcp__server` prefix → return deferred tools whose names begin with it
//   - `+term` required terms → candidate must match all required terms in
//     name parts or description
//   - any other space-separated terms → scored, sorted, top maxResults
//
// Scoring weights purposely mirror Claude Code's so agents sharing a
// mental model of "how does ToolSearch work?" get the same answers.
func searchTools(query string, pool []core.Tool, maxResults int, lookup scoringLookup) []string {
	qLower := strings.TrimSpace(strings.ToLower(query))
	if qLower == "" || len(pool) == 0 {
		return nil
	}
	if lookup == nil {
		lookup = uncachedLookup
	}

	// Partition pool into deferred (for scoring) and the full set (for
	// exact-match fallback).
	var deferred []core.Tool
	for _, t := range pool {
		if t.ShouldDefer {
			deferred = append(deferred, t)
		}
	}

	// `select:Name1,Name2,...` — direct multi-select bypasses scoring.
	// Falls back to the full pool so requesting an already-loaded tool
	// returns a harmless no-op rather than a confusing "no matches".
	if rest, ok := strings.CutPrefix(qLower, "select:"); ok {
		names := splitAndTrim(rest, ",")
		if len(names) == 0 {
			return nil
		}
		byLower := make(map[string]string, len(pool))
		for _, t := range pool {
			byLower[strings.ToLower(t.Definition.Name)] = t.Definition.Name
		}
		var found []string
		seen := make(map[string]bool)
		for _, req := range names {
			if real, ok := byLower[strings.ToLower(req)]; ok && !seen[real] {
				found = append(found, real)
				seen[real] = true
			}
		}
		return found
	}

	// Exact name match (harmless fast path — helps when the model forgets
	// the `select:` prefix; common from post-compaction retries). Check
	// deferred first, then fall back to the full pool — matches Claude
	// Code's behavior of treating an already-loaded match as a no-op.
	for _, t := range deferred {
		if strings.EqualFold(t.Definition.Name, qLower) {
			return []string{t.Definition.Name}
		}
	}
	for _, t := range pool {
		if strings.EqualFold(t.Definition.Name, qLower) {
			return []string{t.Definition.Name}
		}
	}

	// mcp__server prefix search — scored against deferred only, since
	// the whole point of an mcp__ prefix search is discovering tools
	// hidden behind deferral.
	if strings.HasPrefix(qLower, "mcp__") && len(qLower) > 5 {
		var matches []string
		for _, t := range deferred {
			if strings.HasPrefix(strings.ToLower(t.Definition.Name), qLower) {
				matches = append(matches, t.Definition.Name)
				if len(matches) >= maxResults {
					break
				}
			}
		}
		if len(matches) > 0 {
			return matches
		}
	}

	// Split into +required and optional terms.
	var required, optional []string
	for _, term := range strings.Fields(qLower) {
		if len(term) > 1 && strings.HasPrefix(term, "+") {
			required = append(required, term[1:])
		} else {
			optional = append(optional, term)
		}
	}

	var scoringTerms []string
	if len(required) > 0 {
		scoringTerms = append(scoringTerms, required...)
		scoringTerms = append(scoringTerms, optional...)
	} else {
		scoringTerms = append(scoringTerms, strings.Fields(qLower)...)
	}
	patterns := compileTermPatterns(scoringTerms)

	// Pre-filter by required terms. A candidate is kept only if every
	// required term is present in name parts, description, or searchHint.
	candidates := deferred
	if len(required) > 0 {
		var keep []core.Tool
		for _, t := range deferred {
			entry := lookup(t)
			allMatch := true
			for _, term := range required {
				pattern := patterns[term]
				if !hasTermMatchCached(entry, term, pattern) {
					allMatch = false
					break
				}
			}
			if allMatch {
				keep = append(keep, t)
			}
		}
		candidates = keep
	}

	// Score each candidate.
	scored := make([]searchResult, 0, len(candidates))
	for _, t := range candidates {
		entry := lookup(t)
		score := 0

		for _, term := range scoringTerms {
			pattern := patterns[term]

			// Exact part match.
			if containsEqual(entry.parsed.parts, term) {
				if entry.parsed.isMcp {
					score += 12
				} else {
					score += 10
				}
			} else if containsSubstring(entry.parsed.parts, term) {
				if entry.parsed.isMcp {
					score += 6
				} else {
					score += 5
				}
			}

			// Full-name fallback (only when nothing else scored yet).
			if score == 0 && strings.Contains(entry.parsed.full, term) {
				score += 3
			}

			// searchHint match — curated capability phrase.
			if entry.hintLower != "" && pattern.MatchString(entry.hintLower) {
				score += 4
			}

			// Description match (word-boundary).
			if pattern.MatchString(entry.descLower) {
				score += 2
			}
		}

		if score > 0 {
			scored = append(scored, searchResult{name: t.Definition.Name, score: score})
		}
	}

	// Stable sort: primary by score desc, secondary by name asc.
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].name < scored[j].name
	})

	if len(scored) > maxResults {
		scored = scored[:maxResults]
	}

	out := make([]string, 0, len(scored))
	for _, s := range scored {
		out = append(out, s.name)
	}
	return out
}

// hasTermMatchCached reports whether a term is found in any of the
// searchable surfaces (name parts, description, or searchHint) of a
// precomputed scoringEntry.
func hasTermMatchCached(entry scoringEntry, term string, pattern *regexp.Regexp) bool {
	if containsEqual(entry.parsed.parts, term) || containsSubstring(entry.parsed.parts, term) {
		return true
	}
	if pattern.MatchString(entry.descLower) {
		return true
	}
	if entry.hintLower != "" && pattern.MatchString(entry.hintLower) {
		return true
	}
	return false
}

func containsEqual(parts []string, term string) bool {
	for _, p := range parts {
		if p == term {
			return true
		}
	}
	return false
}

func containsSubstring(parts []string, term string) bool {
	for _, p := range parts {
		if strings.Contains(p, term) {
			return true
		}
	}
	return false
}

func splitAndTrim(s, sep string) []string {
	raw := strings.Split(s, sep)
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		r = strings.TrimSpace(r)
		if r != "" {
			out = append(out, r)
		}
	}
	return out
}
