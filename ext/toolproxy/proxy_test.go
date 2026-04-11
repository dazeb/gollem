package toolproxy

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/fugue-labs/gollem/core"
)

// --------------------------------------------------------------------------
// parseToolName
// --------------------------------------------------------------------------

func TestParseToolName_CamelCase(t *testing.T) {
	got := parseToolName("FileEditTool")
	want := []string{"file", "edit", "tool"}
	if !stringSliceEqual(got.parts, want) {
		t.Errorf("CamelCase parts: got %v want %v", got.parts, want)
	}
	if got.isMcp {
		t.Error("CamelCase name should not be marked as MCP")
	}
}

func TestParseToolName_SnakeCase(t *testing.T) {
	got := parseToolName("file_edit_tool")
	want := []string{"file", "edit", "tool"}
	if !stringSliceEqual(got.parts, want) {
		t.Errorf("snake_case parts: got %v want %v", got.parts, want)
	}
}

func TestParseToolName_MCP(t *testing.T) {
	got := parseToolName("mcp__github__create_issue")
	want := []string{"github", "create", "issue"}
	if !stringSliceEqual(got.parts, want) {
		t.Errorf("MCP parts: got %v want %v", got.parts, want)
	}
	if !got.isMcp {
		t.Error("mcp__ prefix should set isMcp=true")
	}
}

// --------------------------------------------------------------------------
// searchTools — scoring
// --------------------------------------------------------------------------

// makeDeferredTool builds a deferred Tool with the given name / description.
func makeDeferredTool(name, description string) core.Tool {
	type EmptyParams struct{}
	tool := core.FuncTool[EmptyParams](name, description,
		func(_ context.Context, _ EmptyParams) (string, error) {
			return "ok", nil
		})
	tool.ShouldDefer = true
	return tool
}

func TestSearchTools_SelectDirectLookup(t *testing.T) {
	tools := []core.Tool{
		makeDeferredTool("Read", "Read files from the filesystem"),
		makeDeferredTool("Edit", "Make exact edits to files"),
		makeDeferredTool("Grep", "Search for patterns in files"),
	}
	got := searchTools("select:Read,Edit", tools, 5, nil)
	want := []string{"Read", "Edit"}
	if !stringSliceEqual(got, want) {
		t.Errorf("select: got %v want %v", got, want)
	}
}

func TestSearchTools_SelectMissingIgnored(t *testing.T) {
	tools := []core.Tool{
		makeDeferredTool("Read", "Read files"),
	}
	got := searchTools("select:Read,NonExistent", tools, 5, nil)
	want := []string{"Read"}
	if !stringSliceEqual(got, want) {
		t.Errorf("missing names should be silently dropped: got %v want %v", got, want)
	}
}

// TestSearchTools_SelectNonDeferredFallback verifies Claude Code's
// no-op fallback: if the model asks tool_search to load a tool that's
// already in the pool as a non-deferred entry, we still return it
// rather than confusing the model with "no matches".
func TestSearchTools_SelectNonDeferredFallback(t *testing.T) {
	type Empty struct{}
	// A deferred tool and a non-deferred tool share the pool.
	nonDeferred := core.FuncTool[Empty]("already_loaded", "Already in the tool list",
		func(_ context.Context, _ Empty) (string, error) { return "", nil })
	tools := []core.Tool{
		makeDeferredTool("hidden", "Deferred tool"),
		nonDeferred,
	}

	got := searchTools("select:already_loaded", tools, 5, nil)
	want := []string{"already_loaded"}
	if !stringSliceEqual(got, want) {
		t.Errorf("select: on a non-deferred tool should fall back as a no-op: got %v want %v", got, want)
	}
}

// TestSearchTools_ExactMatchNonDeferredFallback is the same as above
// but exercises the bare-name (non-`select:`) exact match path.
func TestSearchTools_ExactMatchNonDeferredFallback(t *testing.T) {
	type Empty struct{}
	nonDeferred := core.FuncTool[Empty]("already_loaded", "Already in the tool list",
		func(_ context.Context, _ Empty) (string, error) { return "", nil })
	tools := []core.Tool{
		makeDeferredTool("hidden", "Deferred tool"),
		nonDeferred,
	}

	got := searchTools("already_loaded", tools, 5, nil)
	want := []string{"already_loaded"}
	if !stringSliceEqual(got, want) {
		t.Errorf("bare exact match on a non-deferred tool should fall back: got %v want %v", got, want)
	}
}

func TestSearchTools_KeywordMatch(t *testing.T) {
	tools := []core.Tool{
		makeDeferredTool("FileRead", "Read files from disk"),
		makeDeferredTool("FileWrite", "Write files to disk"),
		makeDeferredTool("WebFetch", "Download a URL from the web"),
	}
	got := searchTools("file read", tools, 5, nil)
	// Both File* tools match "file" and FileRead matches "read" too, so it
	// should rank above FileWrite.
	if len(got) == 0 {
		t.Fatal("expected at least one match")
	}
	if got[0] != "FileRead" {
		t.Errorf("FileRead should be top match: got %v", got)
	}
}

func TestSearchTools_RequiredTerm(t *testing.T) {
	tools := []core.Tool{
		makeDeferredTool("SlackSend", "Send a message to Slack"),
		makeDeferredTool("DiscordSend", "Send a message to Discord"),
		makeDeferredTool("SlackRead", "Read a Slack channel"),
	}
	// +slack forces slack to be present, send is the scoring term.
	got := searchTools("+slack send", tools, 5, nil)
	if len(got) == 0 {
		t.Fatal("expected matches for +slack send")
	}
	// Discord variant should not appear.
	for _, name := range got {
		if strings.Contains(strings.ToLower(name), "discord") {
			t.Errorf("Discord tool should be excluded by +slack: got %v", got)
		}
	}
	// SlackSend should beat SlackRead.
	if got[0] != "SlackSend" {
		t.Errorf("SlackSend should rank first: got %v", got)
	}
}

func TestSearchTools_MaxResults(t *testing.T) {
	tools := []core.Tool{
		makeDeferredTool("FileRead", "Read files"),
		makeDeferredTool("FileWrite", "Write files"),
		makeDeferredTool("FileEdit", "Edit files"),
		makeDeferredTool("FileDelete", "Delete files"),
	}
	got := searchTools("file", tools, 2, nil)
	if len(got) > 2 {
		t.Errorf("max_results=2 should cap at 2: got %d results", len(got))
	}
}

func TestSearchTools_MCPPrefixScoring(t *testing.T) {
	tools := []core.Tool{
		makeDeferredTool("mcp__github__create_issue", "Create a GitHub issue"),
		makeDeferredTool("mcp__slack__send_message", "Send a Slack message"),
	}
	got := searchTools("github issue", tools, 5, nil)
	if len(got) == 0 || got[0] != "mcp__github__create_issue" {
		t.Errorf("expected mcp__github__create_issue on top, got %v", got)
	}
}

func TestSearchTools_SearchHintBoost(t *testing.T) {
	tools := []core.Tool{
		makeDeferredTool("Alpha", "Generic description"),
		makeDeferredTool("Beta", "Generic description"),
	}
	// Add a curated hint to Beta that should pull it to the top for
	// "deploy" even though the name and description say nothing about it.
	tools[1].SearchHint = "deploy production services"
	got := searchTools("deploy", tools, 5, nil)
	if len(got) == 0 {
		t.Fatal("expected match via searchHint")
	}
	if got[0] != "Beta" {
		t.Errorf("Beta should rank first via searchHint: got %v", got)
	}
}

// --------------------------------------------------------------------------
// state: ExportState / RestoreState
// --------------------------------------------------------------------------

func TestDiscoveredState_ExportRestore(t *testing.T) {
	s := newDiscoveredState()
	s.add("Read")
	s.add("Edit")
	s.add("Read") // duplicate, should be idempotent

	exported, err := s.ExportState()
	if err != nil {
		t.Fatalf("ExportState: %v", err)
	}

	// Roundtrip through JSON so we hit the same path checkpoints take.
	data, err := json.Marshal(exported)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var roundtripped any
	if err := json.Unmarshal(data, &roundtripped); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	restored := newDiscoveredState()
	if err := restored.RestoreState(roundtripped); err != nil {
		t.Fatalf("RestoreState: %v", err)
	}

	got := restored.snapshot()
	want := []string{"Edit", "Read"}
	if !stringSliceEqual(got, want) {
		t.Errorf("roundtrip: got %v want %v", got, want)
	}
}

func TestDiscoveredState_RestoreFromStringSlice(t *testing.T) {
	s := newDiscoveredState()
	// Direct shape (pre-JSON) — should work too.
	if err := s.RestoreState([]string{"A", "B", "C"}); err != nil {
		t.Fatalf("RestoreState: %v", err)
	}
	got := s.snapshot()
	want := []string{"A", "B", "C"}
	if !stringSliceEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

// --------------------------------------------------------------------------
// PrepareFuncFor — filter behavior
// --------------------------------------------------------------------------

// setup builds an agent-like pool: N non-deferred tools + M deferred tools
// + the tool_search tool from a Proxy.
func setup(t *testing.T, mode Mode, nonDefer, defer_ int) (*Proxy, []core.Tool, []core.ToolDefinition) {
	t.Helper()
	p := New(Config{Mode: mode})

	var tools []core.Tool
	for i := 0; i < nonDefer; i++ {
		type EmptyParams struct{}
		tools = append(tools, core.FuncTool[EmptyParams](
			"keep_"+string(rune('A'+i)),
			"Non-deferred tool",
			func(_ context.Context, _ EmptyParams) (string, error) { return "ok", nil },
		))
	}
	for i := 0; i < defer_; i++ {
		tools = append(tools, makeDeferredTool(
			"hide_"+string(rune('A'+i)),
			"Deferred tool with a moderate-length description to make it "+
				"contribute meaningfully to the character heuristic used in auto mode.",
		))
	}
	tools = append(tools, p.Tool())

	defs := make([]core.ToolDefinition, len(tools))
	for i, t := range tools {
		defs[i] = t.Definition
	}
	return p, tools, defs
}

func TestPrepareFuncFor_AlwaysMode_HidesDeferred(t *testing.T) {
	p, tools, defs := setup(t, ModeAlways, 2, 3)
	pf := p.PrepareFuncFor(tools)

	got := pf(context.Background(), nil, defs)
	gotNames := namesFromDefs(got)

	// Expect: 2 keep_* + tool_search (the 3 hide_* are dropped).
	wantContains := []string{"keep_A", "keep_B", DefaultToolName}
	wantMissing := []string{"hide_A", "hide_B", "hide_C"}

	for _, name := range wantContains {
		if !containsString(gotNames, name) {
			t.Errorf("expected %q in filtered list, got %v", name, gotNames)
		}
	}
	for _, name := range wantMissing {
		if containsString(gotNames, name) {
			t.Errorf("did not expect %q in filtered list, got %v", name, gotNames)
		}
	}
}

func TestPrepareFuncFor_AutoMode_BelowThresholdPassThrough(t *testing.T) {
	// Small pool: 2 keeps + 1 deferred. Way under 10% of 200k context.
	p, tools, defs := setup(t, ModeAuto, 2, 1)
	pf := p.PrepareFuncFor(tools)

	got := pf(context.Background(), nil, defs)
	gotNames := namesFromDefs(got)

	// All three input tools should survive.
	want := []string{"keep_A", "keep_B", "hide_A", DefaultToolName}
	for _, n := range want {
		if !containsString(gotNames, n) {
			t.Errorf("pass-through mode should include %q; got %v", n, gotNames)
		}
	}
}

func TestPrepareFuncFor_AutoMode_AboveThresholdDefers(t *testing.T) {
	// Force threshold to ~1 token so any deferred tool trips it.
	p := New(Config{
		Mode:           ModeAuto,
		AutoTokenRatio: 0.0001,
		ContextWindow:  1000,
	})
	tools := []core.Tool{
		makeDeferredTool("hide_A", strings.Repeat("long description ", 50)),
		makeDeferredTool("hide_B", strings.Repeat("long description ", 50)),
	}
	type EmptyParams struct{}
	tools = append(tools, core.FuncTool[EmptyParams]("keep_A", "short",
		func(_ context.Context, _ EmptyParams) (string, error) { return "ok", nil }))
	tools = append(tools, p.Tool())

	defs := make([]core.ToolDefinition, len(tools))
	for i, t := range tools {
		defs[i] = t.Definition
	}

	pf := p.PrepareFuncFor(tools)
	got := pf(context.Background(), nil, defs)
	gotNames := namesFromDefs(got)

	if containsString(gotNames, "hide_A") || containsString(gotNames, "hide_B") {
		t.Errorf("above-threshold auto mode should defer: got %v", gotNames)
	}
	if !containsString(gotNames, "keep_A") {
		t.Errorf("non-deferred tool must remain: got %v", gotNames)
	}
	if !containsString(gotNames, DefaultToolName) {
		t.Errorf("tool_search must remain: got %v", gotNames)
	}
}

// --------------------------------------------------------------------------
// End-to-end: Agent → tool_search → discover → next request includes tool
// --------------------------------------------------------------------------

// TestAgentEndToEnd_DiscoveryFlow runs a real agent loop with TestModel to
// verify the three-step cycle:
//  1. First request sees tool_search + non-deferred tools, NOT hide_*.
//  2. Model calls tool_search(select:hide_target).
//  3. Second request sees tool_search + non-deferred + hide_target.
func TestAgentEndToEnd_DiscoveryFlow(t *testing.T) {
	proxy := New(Config{Mode: ModeAlways})

	type EmptyParams struct{}
	keepTool := core.FuncTool[EmptyParams]("keep_me", "A non-deferred tool",
		func(_ context.Context, _ EmptyParams) (string, error) { return "kept", nil })

	hideA := makeDeferredTool("hide_A", "Deferred tool A")
	hideB := makeDeferredTool("hide_B", "Deferred tool B that we will load")
	hideC := makeDeferredTool("hide_C", "Deferred tool C")

	tools := []core.Tool{keepTool, hideA, hideB, hideC, proxy.Tool()}

	// Canned model responses: step 1 calls tool_search, step 2 calls hide_B,
	// step 3 stops with text.
	model := core.NewTestModel(
		core.ToolCallResponseWithID(DefaultToolName, `{"query":"select:hide_B"}`, "call_1"),
		core.ToolCallResponseWithID("hide_B", `{}`, "call_2"),
		core.TextResponse("done"),
	)

	agent := core.NewAgent[string](model,
		core.WithTools[string](tools...),
		core.WithToolsPrepare[string](proxy.PrepareFuncFor(tools)),
	)

	_, err := agent.Run(context.Background(), "do the thing")
	if err != nil {
		t.Fatalf("agent run failed: %v", err)
	}

	calls := model.Calls()
	if len(calls) < 3 {
		t.Fatalf("expected 3 model calls, got %d", len(calls))
	}

	// --- First request: hide_* must NOT be present.
	firstNames := namesFromDefs(calls[0].Parameters.FunctionTools)
	if containsString(firstNames, "hide_A") || containsString(firstNames, "hide_B") || containsString(firstNames, "hide_C") {
		t.Errorf("first request should not contain hide_* tools: got %v", firstNames)
	}
	if !containsString(firstNames, "keep_me") {
		t.Errorf("first request should contain non-deferred tools: got %v", firstNames)
	}
	if !containsString(firstNames, DefaultToolName) {
		t.Errorf("first request should contain tool_search: got %v", firstNames)
	}

	// --- Second request: after tool_search(select:hide_B), hide_B must appear.
	secondNames := namesFromDefs(calls[1].Parameters.FunctionTools)
	if !containsString(secondNames, "hide_B") {
		t.Errorf("second request should include hide_B after discovery: got %v", secondNames)
	}
	if containsString(secondNames, "hide_A") || containsString(secondNames, "hide_C") {
		t.Errorf("undiscovered hide_* tools should stay hidden: got %v", secondNames)
	}

	// --- Third request: after hide_B executes, it should remain available.
	// Discovery is sticky for the lifetime of the run — once a deferred
	// tool is loaded, subsequent turns still see it. This catches a
	// regression where we'd drop the discovered set between turns.
	thirdNames := namesFromDefs(calls[2].Parameters.FunctionTools)
	if !containsString(thirdNames, "hide_B") {
		t.Errorf("third request should still include hide_B: got %v", thirdNames)
	}
	if containsString(thirdNames, "hide_A") || containsString(thirdNames, "hide_C") {
		t.Errorf("undiscovered hide_* tools should still be hidden on turn 3: got %v", thirdNames)
	}

	// Proxy's public discovered set should reflect the load.
	discovered := proxy.Discovered()
	if len(discovered) != 1 || discovered[0] != "hide_B" {
		t.Errorf("Discovered() should report [hide_B], got %v", discovered)
	}
}

// TestAgentEndToEnd_ZeroValueConfigDefaultsToAlwaysMode verifies that
// `New(Config{})` — the zero-value config — defers tools immediately,
// matching Claude Code's `tst` default.
func TestAgentEndToEnd_ZeroValueConfigDefaultsToAlwaysMode(t *testing.T) {
	proxy := New(Config{}) // ModeAlways is the zero value

	type Empty struct{}
	keepTool := core.FuncTool[Empty]("keep_me", "A non-deferred tool",
		func(_ context.Context, _ Empty) (string, error) { return "kept", nil })
	hide := makeDeferredTool("hide_me", "Deferred tool")
	tools := []core.Tool{keepTool, hide, proxy.Tool()}

	model := core.NewTestModel(core.TextResponse("done"))
	agent := core.NewAgent[string](model,
		core.WithTools[string](tools...),
		core.WithToolsPrepare[string](proxy.PrepareFuncFor(tools)),
	)

	_, err := agent.Run(context.Background(), "go")
	if err != nil {
		t.Fatalf("agent run failed: %v", err)
	}

	calls := model.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 model call, got %d", len(calls))
	}

	names := namesFromDefs(calls[0].Parameters.FunctionTools)
	if containsString(names, "hide_me") {
		t.Errorf("zero-value Config should default to ModeAlways and defer hide_me: got %v", names)
	}
	if !containsString(names, "keep_me") || !containsString(names, DefaultToolName) {
		t.Errorf("zero-value Config should still expose keep_me and tool_search: got %v", names)
	}
}

// TestScoringCache_HitAndInvalidate verifies that the per-Proxy
// scoring cache (a) returns the same entry for repeated lookups of the
// same tool, and (b) is dropped when setPool observes a pool with a
// different signature. Without this guard the cache would return stale
// parsed names after a pool swap.
func TestScoringCache_HitAndInvalidate(t *testing.T) {
	p := New(Config{})

	tool := makeDeferredTool("CacheMe", "A tool under test")

	// Prime the cache and verify we get consistent results.
	entry1 := p.state.lookupScoring(tool)
	entry2 := p.state.lookupScoring(tool)
	if entry1.descLower != entry2.descLower || entry1.hintLower != entry2.hintLower {
		t.Errorf("repeated lookups must return the same entry: %+v vs %+v", entry1, entry2)
	}

	// Record a pool that includes CacheMe — cache survives (same signature).
	p.state.setPool([]core.Tool{tool})
	p.state.lookupScoring(tool) // populate
	if len(p.state.scoringCache) != 1 {
		t.Errorf("after lookup, cache should have 1 entry: got %d", len(p.state.scoringCache))
	}

	// Setting an equivalent pool (same names) must NOT invalidate.
	p.state.setPool([]core.Tool{tool})
	if p.state.scoringCache == nil || len(p.state.scoringCache) != 1 {
		t.Errorf("identical pool signature must preserve cache: got %v", p.state.scoringCache)
	}

	// Setting a DIFFERENT pool must invalidate.
	other := makeDeferredTool("Different", "Another tool")
	p.state.setPool([]core.Tool{other})
	if p.state.scoringCache != nil {
		t.Errorf("pool signature change must clear cache: got %v", p.state.scoringCache)
	}
}

// TestScoringCache_NonDeferredPoolChangePreservesCache makes sure the
// cache only invalidates when the DEFERRED subset changes. Adding or
// removing a non-deferred tool should keep cached entries warm, since
// scoring never touches non-deferred tools anyway.
func TestScoringCache_NonDeferredPoolChangePreservesCache(t *testing.T) {
	p := New(Config{})
	hide := makeDeferredTool("hide", "deferred")

	type Empty struct{}
	keep := core.FuncTool[Empty]("keep", "non-deferred",
		func(_ context.Context, _ Empty) (string, error) { return "", nil })

	p.state.setPool([]core.Tool{hide, keep})
	p.state.lookupScoring(hide) // populate

	if len(p.state.scoringCache) != 1 {
		t.Fatalf("precondition: cache should have 1 entry, got %d", len(p.state.scoringCache))
	}

	// Add another non-deferred tool — deferred set is unchanged, so
	// the signature must not change and the cache must survive.
	keep2 := core.FuncTool[Empty]("keep_two", "also non-deferred",
		func(_ context.Context, _ Empty) (string, error) { return "", nil })
	p.state.setPool([]core.Tool{hide, keep, keep2})

	if p.state.scoringCache == nil || len(p.state.scoringCache) != 1 {
		t.Errorf("non-deferred addition should preserve cache; got %v", p.state.scoringCache)
	}

	// Remove a non-deferred tool — same story.
	p.state.setPool([]core.Tool{hide})
	if p.state.scoringCache == nil || len(p.state.scoringCache) != 1 {
		t.Errorf("non-deferred removal should preserve cache; got %v", p.state.scoringCache)
	}
}

// TestStatefulToolCheckpointRoundtrip verifies that the discovered set
// survives a full ExportState → serialize → deserialize → RestoreState
// cycle on the Tool struct returned by Proxy.Tool().
func TestStatefulToolCheckpointRoundtrip(t *testing.T) {
	p1 := New(Config{})
	_ = p1.state.add("hide_A")
	_ = p1.state.add("hide_B")

	stateful := p1.Tool().Stateful
	if stateful == nil {
		t.Fatal("Proxy.Tool().Stateful must not be nil — discovered set won't survive checkpoints")
	}

	exported, err := stateful.ExportState()
	if err != nil {
		t.Fatalf("ExportState failed: %v", err)
	}

	// Roundtrip through JSON (how gollem persists checkpoint state).
	data, err := json.Marshal(exported)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var roundtripped any
	if err := json.Unmarshal(data, &roundtripped); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	// Fresh proxy to simulate a reloaded run.
	p2 := New(Config{})
	if err := p2.Tool().Stateful.RestoreState(roundtripped); err != nil {
		t.Fatalf("RestoreState failed: %v", err)
	}

	got := p2.Discovered()
	want := []string{"hide_A", "hide_B"}
	if !stringSliceEqual(got, want) {
		t.Errorf("after checkpoint roundtrip: got %v want %v", got, want)
	}
}

// TestSystemPromptFragment emits a deferred-tools list the model can read.
func TestSystemPromptFragment(t *testing.T) {
	p := New(Config{})
	tools := []core.Tool{
		makeDeferredTool("hide_X", "X"),
		makeDeferredTool("hide_Y", "Y"),
	}
	frag := p.SystemPromptFragment(tools)

	if !strings.Contains(frag, "hide_X") || !strings.Contains(frag, "hide_Y") {
		t.Errorf("fragment should list deferred tool names: %q", frag)
	}
	if !strings.Contains(frag, DefaultToolName) {
		t.Errorf("fragment should mention tool_search tool name: %q", frag)
	}
}

func TestSystemPromptFragment_EmptyWhenNoDeferredTools(t *testing.T) {
	p := New(Config{})
	type EmptyParams struct{}
	tools := []core.Tool{
		core.FuncTool[EmptyParams]("keep", "",
			func(_ context.Context, _ EmptyParams) (string, error) { return "", nil }),
	}
	if frag := p.SystemPromptFragment(tools); frag != "" {
		t.Errorf("no deferred tools → empty fragment; got %q", frag)
	}
}

// --------------------------------------------------------------------------
// MarkDeferred / MarkDeferredIf helpers
// --------------------------------------------------------------------------

func TestMarkDeferred_StampsAllTools(t *testing.T) {
	type Empty struct{}
	in := []core.Tool{
		core.FuncTool[Empty]("alpha", "",
			func(_ context.Context, _ Empty) (string, error) { return "", nil }),
		core.FuncTool[Empty]("beta", "",
			func(_ context.Context, _ Empty) (string, error) { return "", nil }),
	}

	out := MarkDeferred(in)
	if len(out) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(out))
	}
	for _, tool := range out {
		if !tool.ShouldDefer {
			t.Errorf("tool %q should be marked deferred", tool.Definition.Name)
		}
	}

	// Original slice must not be mutated.
	for _, tool := range in {
		if tool.ShouldDefer {
			t.Errorf("input slice should not be mutated: %q has ShouldDefer=true", tool.Definition.Name)
		}
	}
}

func TestMarkDeferred_PreservesOtherFields(t *testing.T) {
	type Empty struct{}
	tool := core.FuncTool[Empty]("alpha", "desc",
		func(_ context.Context, _ Empty) (string, error) { return "", nil })
	tool.SearchHint = "important hint"
	tool.RequiresApproval = true

	out := MarkDeferred([]core.Tool{tool})
	if out[0].SearchHint != "important hint" {
		t.Errorf("SearchHint should be preserved, got %q", out[0].SearchHint)
	}
	if !out[0].RequiresApproval {
		t.Error("RequiresApproval should be preserved")
	}
	if !out[0].ShouldDefer {
		t.Error("ShouldDefer should be set")
	}
}

func TestMarkDeferredIf_RespectsPredicate(t *testing.T) {
	type Empty struct{}
	in := []core.Tool{
		core.FuncTool[Empty]("mcp__github__create_issue", "",
			func(_ context.Context, _ Empty) (string, error) { return "", nil }),
		core.FuncTool[Empty]("mcp__slack__send", "",
			func(_ context.Context, _ Empty) (string, error) { return "", nil }),
		core.FuncTool[Empty]("local_tool", "",
			func(_ context.Context, _ Empty) (string, error) { return "", nil }),
	}

	out := MarkDeferredIf(in, func(t core.Tool) bool {
		return strings.HasPrefix(t.Definition.Name, "mcp__")
	})

	wantDeferred := map[string]bool{
		"mcp__github__create_issue": true,
		"mcp__slack__send":          true,
		"local_tool":                false,
	}
	for _, tool := range out {
		if tool.ShouldDefer != wantDeferred[tool.Definition.Name] {
			t.Errorf("%q: got ShouldDefer=%v want %v",
				tool.Definition.Name, tool.ShouldDefer, wantDeferred[tool.Definition.Name])
		}
	}
}

func TestMarkDeferred_EmptyInput(t *testing.T) {
	if out := MarkDeferred(nil); out != nil {
		t.Errorf("nil input should return nil, got %v", out)
	}
	if out := MarkDeferred([]core.Tool{}); out != nil {
		t.Errorf("empty input should return nil, got %v", out)
	}
}

// TestMarkDeferred_SkipsAlwaysLoad verifies that MarkDeferred respects
// a pre-existing AlwaysLoad=true opt-out: the tool retains its inline
// status even when the stamper would otherwise mark it.
func TestMarkDeferred_SkipsAlwaysLoad(t *testing.T) {
	type Empty struct{}
	critical := core.FuncTool[Empty]("abort", "",
		func(_ context.Context, _ Empty) (string, error) { return "", nil })
	critical.AlwaysLoad = true

	other := core.FuncTool[Empty]("normal", "",
		func(_ context.Context, _ Empty) (string, error) { return "", nil })

	out := MarkDeferred([]core.Tool{critical, other})

	var gotCritical, gotOther core.Tool
	for _, t := range out {
		switch t.Definition.Name {
		case "abort":
			gotCritical = t
		case "normal":
			gotOther = t
		}
	}

	if gotCritical.ShouldDefer {
		t.Error("AlwaysLoad tool should not be stamped ShouldDefer")
	}
	if !gotCritical.AlwaysLoad {
		t.Error("AlwaysLoad flag should be preserved")
	}
	if !gotOther.ShouldDefer {
		t.Error("non-AlwaysLoad tool should be stamped ShouldDefer")
	}
}

// TestPrepareFuncFor_AlwaysLoadBypassesDeferral verifies a tool with
// both ShouldDefer=true and AlwaysLoad=true is included in the request
// even when it has not been discovered. AlwaysLoad is the escape hatch
// for "must be available every turn" tools.
func TestPrepareFuncFor_AlwaysLoadBypassesDeferral(t *testing.T) {
	p := New(Config{})
	critical := makeDeferredTool("critical", "Must be always available")
	critical.AlwaysLoad = true
	hidden := makeDeferredTool("hidden", "Normal deferred")

	tools := []core.Tool{critical, hidden, p.Tool()}
	defs := make([]core.ToolDefinition, len(tools))
	for i, t := range tools {
		defs[i] = t.Definition
	}

	got := p.PrepareFuncFor(tools)(context.Background(), nil, defs)
	names := namesFromDefs(got)

	if !containsString(names, "critical") {
		t.Errorf("AlwaysLoad tool must always be included, got %v", names)
	}
	if containsString(names, "hidden") {
		t.Errorf("plain deferred tool should be dropped, got %v", names)
	}
}

// TestSystemPromptFragment_SkipsAlwaysLoad verifies the static
// fragment builder excludes AlwaysLoad tools (they're already inline
// so listing them in the deferred section would be misleading).
func TestSystemPromptFragment_SkipsAlwaysLoad(t *testing.T) {
	p := New(Config{})
	hide := makeDeferredTool("hide", "Deferred")
	critical := makeDeferredTool("critical", "AlwaysLoad")
	critical.AlwaysLoad = true

	frag := p.SystemPromptFragment([]core.Tool{hide, critical})
	if !strings.Contains(frag, "hide") {
		t.Errorf("fragment should list hide, got %q", frag)
	}
	if strings.Contains(frag, "critical") {
		t.Errorf("fragment should not list AlwaysLoad tool 'critical', got %q", frag)
	}
}

// --------------------------------------------------------------------------
// Delta announcement: WrapPrompt + SystemPromptFuncFor
// --------------------------------------------------------------------------

func TestWrapPrompt_PrependsFragment(t *testing.T) {
	p := New(Config{})
	tools := []core.Tool{
		makeDeferredTool("hide_A", "Deferred A"),
		makeDeferredTool("hide_B", "Deferred B"),
	}
	prompt := "do the thing"
	got := p.WrapPrompt(prompt, tools)

	if !strings.Contains(got, "hide_A") || !strings.Contains(got, "hide_B") {
		t.Errorf("wrapped prompt should list deferred tool names, got %q", got)
	}
	if !strings.HasSuffix(got, prompt) {
		t.Errorf("wrapped prompt should end with the original prompt, got %q", got)
	}
	if !strings.Contains(got, DefaultToolName) {
		t.Errorf("wrapped prompt should mention the tool_search tool name, got %q", got)
	}
}

func TestWrapPrompt_EmptyWithNoDeferredTools(t *testing.T) {
	p := New(Config{})
	type Empty struct{}
	tools := []core.Tool{
		core.FuncTool[Empty]("keep", "",
			func(_ context.Context, _ Empty) (string, error) { return "", nil }),
	}
	prompt := "do the thing"
	got := p.WrapPrompt(prompt, tools)
	if got != prompt {
		t.Errorf("no deferred tools → prompt unchanged; got %q", got)
	}
}

func TestWrapPrompt_ModeOffPassThrough(t *testing.T) {
	p := New(Config{Mode: ModeOff})
	tools := []core.Tool{makeDeferredTool("hide_A", "Deferred")}
	got := p.WrapPrompt("do", tools)
	if got != "do" {
		t.Errorf("ModeOff should pass the prompt through unchanged: got %q", got)
	}
}

func TestSystemPromptFuncFor_EmitsOnceThenEmpty(t *testing.T) {
	p := New(Config{})
	tools := []core.Tool{
		makeDeferredTool("hide_A", "Deferred A"),
		makeDeferredTool("hide_B", "Deferred B"),
	}
	fn := p.SystemPromptFuncFor(tools)

	// First call → full fragment.
	got1, err := fn(context.Background(), nil)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if !strings.Contains(got1, "hide_A") || !strings.Contains(got1, "hide_B") {
		t.Errorf("first call should emit full fragment; got %q", got1)
	}

	// Second, third calls → empty (delta).
	got2, _ := fn(context.Background(), nil)
	if got2 != "" {
		t.Errorf("second call should emit empty (delta); got %q", got2)
	}
	got3, _ := fn(context.Background(), nil)
	if got3 != "" {
		t.Errorf("third call should emit empty (delta); got %q", got3)
	}

	// Reset re-arms the announcer.
	p.Reset()
	got4, _ := fn(context.Background(), nil)
	if !strings.Contains(got4, "hide_A") {
		t.Errorf("after Reset, first call should re-announce; got %q", got4)
	}
}

// TestSystemPromptFuncFor_DeltaEmitsOnlyAdditions verifies the new
// delta semantics: when a pool changes mid-run and the caller binds a
// fresh SystemPromptFuncFor to the new pool, the next call emits
// ONLY the newly-deferred names, not the full list again. This is the
// core token-savings property of delta announcement.
func TestSystemPromptFuncFor_DeltaEmitsOnlyAdditions(t *testing.T) {
	p := New(Config{})

	// Turn 1: initial pool announces hide_A.
	initial := []core.Tool{makeDeferredTool("hide_A", "A")}
	fn1 := p.SystemPromptFuncFor(initial)
	got1, err := fn1(context.Background(), nil)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if !strings.Contains(got1, "hide_A") {
		t.Errorf("first call should announce hide_A, got %q", got1)
	}
	if strings.Contains(got1, "Newly Available") {
		t.Errorf("initial announcement should use 'Deferred Tools' header, got %q", got1)
	}

	// Turn 2: same pool, same Proxy → nothing new.
	fn2 := p.SystemPromptFuncFor(initial)
	got2, _ := fn2(context.Background(), nil)
	if got2 != "" {
		t.Errorf("unchanged pool should yield empty delta, got %q", got2)
	}

	// Turn 3: pool grows. New SystemPromptFuncFor closure over the
	// updated tool list emits ONLY the newly added name.
	updated := []core.Tool{
		makeDeferredTool("hide_A", "A"),
		makeDeferredTool("hide_B", "B"),
		makeDeferredTool("hide_C", "C"),
	}
	fn3 := p.SystemPromptFuncFor(updated)
	got3, _ := fn3(context.Background(), nil)

	if !strings.Contains(got3, "hide_B") || !strings.Contains(got3, "hide_C") {
		t.Errorf("delta should list hide_B and hide_C, got %q", got3)
	}
	if strings.Contains(got3, "hide_A") {
		t.Errorf("delta should NOT re-announce already-announced hide_A, got %q", got3)
	}
	if !strings.Contains(got3, "Newly Available") {
		t.Errorf("delta announcement should use 'Newly Available' header, got %q", got3)
	}

	// Turn 4: same (updated) pool → empty again.
	fn4 := p.SystemPromptFuncFor(updated)
	got4, _ := fn4(context.Background(), nil)
	if got4 != "" {
		t.Errorf("after delta, subsequent call should be empty; got %q", got4)
	}
}

// TestSystemPromptFuncFor_DeltaTracksRemovals exercises the removal
// branch: a tool disappears from the deferred pool and the announcer
// should log the removal via PoolChangeEvent but emit no fragment text
// (removals are informational; the model just stops seeing the tool).
func TestSystemPromptFuncFor_DeltaTracksRemovals(t *testing.T) {
	p := New(Config{})
	initial := []core.Tool{
		makeDeferredTool("hide_A", "A"),
		makeDeferredTool("hide_B", "B"),
	}
	fn1 := p.SystemPromptFuncFor(initial)
	if _, err := fn1(context.Background(), nil); err != nil {
		t.Fatalf("first call: %v", err)
	}

	// Pool shrinks — hide_B goes away.
	shrunk := []core.Tool{makeDeferredTool("hide_A", "A")}
	bus := core.NewEventBus()
	var events []PoolChangeEvent
	var mu sync.Mutex
	core.Subscribe(bus, func(e PoolChangeEvent) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})

	rc := &core.RunContext{EventBus: bus}
	fn2 := p.SystemPromptFuncFor(shrunk)
	got, _ := fn2(context.Background(), rc)
	if got != "" {
		t.Errorf("removal-only delta should produce no fragment, got %q", got)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 1 {
		t.Fatalf("expected 1 PoolChangeEvent, got %d", len(events))
	}
	if len(events[0].Added) != 0 {
		t.Errorf("Added should be empty for removal-only delta, got %v", events[0].Added)
	}
	if len(events[0].Removed) != 1 || events[0].Removed[0] != "hide_B" {
		t.Errorf("Removed should be [hide_B], got %v", events[0].Removed)
	}
	if events[0].Initial {
		t.Error("delta after initial announcement should have Initial=false")
	}
}

// TestAgentEndToEnd_SystemPromptFuncForDelta drives a 3-turn agent with
// a dynamic system prompt wired through SystemPromptFuncFor and asserts
// that only the first request's composed system prompt contains the
// deferred-tools fragment.
func TestAgentEndToEnd_SystemPromptFuncForDelta(t *testing.T) {
	proxy := New(Config{})
	hide := makeDeferredTool("hide_me", "A deferred tool named hide_me")
	tools := []core.Tool{hide, proxy.Tool()}

	model := core.NewTestModel(
		core.ToolCallResponseWithID(DefaultToolName, `{"query":"select:hide_me"}`, "call_1"),
		core.ToolCallResponseWithID("hide_me", `{}`, "call_2"),
		core.TextResponse("done"),
	)
	agent := core.NewAgent[string](model,
		core.WithTools[string](tools...),
		core.WithToolsPrepare[string](proxy.PrepareFuncFor(tools)),
		core.WithSystemPrompt[string]("base prompt"),
		core.WithDynamicSystemPrompt[string](proxy.SystemPromptFuncFor(tools)),
	)

	if _, err := agent.Run(context.Background(), "go"); err != nil {
		t.Fatalf("agent run: %v", err)
	}

	calls := model.Calls()
	if len(calls) < 2 {
		t.Fatalf("expected at least 2 calls, got %d", len(calls))
	}

	// currentTurnSystemPrompt returns the concatenated SystemPromptPart
	// content from the LAST ModelRequest in the messages slice — i.e.
	// the new system prompt emitted specifically for this turn. This
	// is the right thing to inspect when checking whether the dynamic
	// prompt func added anything fresh. Earlier turns' ModelRequests
	// naturally remain in the history (that's how the model remembers
	// the first-turn announcement) but don't count as "this turn's"
	// system content.
	currentTurnSystemPrompt := func(call core.TestModelCall) string {
		var lastReqParts []core.ModelRequestPart
		for _, msg := range call.Messages {
			if req, ok := msg.(core.ModelRequest); ok {
				lastReqParts = req.Parts
			}
		}
		var b strings.Builder
		for _, part := range lastReqParts {
			if sp, ok := part.(core.SystemPromptPart); ok {
				if b.Len() > 0 {
					b.WriteByte('\n')
				}
				b.WriteString(sp.Content)
			}
		}
		return b.String()
	}

	// historyHasDeferredHeader walks the full message history for the
	// call and returns true if any ModelRequest contains a
	// SystemPromptPart mentioning the Deferred Tools header. Used to
	// verify the fragment was emitted at least once.
	historyHasDeferredHeader := func(call core.TestModelCall) bool {
		for _, msg := range call.Messages {
			req, ok := msg.(core.ModelRequest)
			if !ok {
				continue
			}
			for _, part := range req.Parts {
				if sp, ok := part.(core.SystemPromptPart); ok {
					if strings.Contains(sp.Content, "Deferred Tools") {
						return true
					}
				}
			}
		}
		return false
	}

	// Turn 1: the fresh SystemPromptPart must contain the fragment.
	first := currentTurnSystemPrompt(calls[0])
	if !strings.Contains(first, "hide_me") {
		t.Errorf("turn 1 current system prompt should contain hide_me, got %q", first)
	}
	if !strings.Contains(first, "Deferred Tools") {
		t.Errorf("turn 1 current system prompt should contain the deferred tools header, got %q", first)
	}

	// Turn 2+: the NEW SystemPromptPart must NOT repeat the fragment
	// (delta), but the history must still carry turn 1's fragment so
	// the model retains context.
	for i, call := range calls[1:] {
		current := currentTurnSystemPrompt(call)
		if strings.Contains(current, "Deferred Tools") {
			t.Errorf("turn %d's new system prompt should not repeat the deferred tools header, got %q", i+2, current)
		}
		if !historyHasDeferredHeader(call) {
			t.Errorf("turn %d's message history should still carry the turn-1 fragment for model recall", i+2)
		}
	}
}

// --------------------------------------------------------------------------
// Telemetry via EventBus
// --------------------------------------------------------------------------

// TestTelemetry_SearchOutcomeEventFiresOnToolSearchCall drives a
// single agent turn in which the model calls tool_search, and
// asserts a SearchOutcomeEvent lands with the expected fields.
func TestTelemetry_SearchOutcomeEventFiresOnToolSearchCall(t *testing.T) {
	proxy := New(Config{})
	bus := core.NewEventBus()

	hideA := makeDeferredTool("hide_A", "Deferred A")
	hideB := makeDeferredTool("hide_B", "Deferred B")
	hideC := makeDeferredTool("hide_C", "Deferred C")
	tools := []core.Tool{hideA, hideB, hideC, proxy.Tool()}

	var outcomes []SearchOutcomeEvent
	var mu sync.Mutex
	core.Subscribe(bus, func(e SearchOutcomeEvent) {
		mu.Lock()
		outcomes = append(outcomes, e)
		mu.Unlock()
	})

	model := core.NewTestModel(
		core.ToolCallResponseWithID(DefaultToolName, `{"query":"select:hide_B,hide_C"}`, "call_1"),
		core.TextResponse("done"),
	)
	agent := core.NewAgent[string](model,
		core.WithTools[string](tools...),
		core.WithToolsPrepare[string](proxy.PrepareFuncFor(tools)),
		core.WithEventBus[string](bus),
	)

	if _, err := agent.Run(context.Background(), "go"); err != nil {
		t.Fatalf("agent run: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(outcomes) != 1 {
		t.Fatalf("expected 1 SearchOutcomeEvent, got %d", len(outcomes))
	}
	out := outcomes[0]
	if out.ToolName != DefaultToolName {
		t.Errorf("ToolName: got %q want %q", out.ToolName, DefaultToolName)
	}
	if out.Query != "select:hide_B,hide_C" {
		t.Errorf("Query: got %q", out.Query)
	}
	if out.QueryKind != SearchQueryKindSelect {
		t.Errorf("QueryKind: got %q want %q", out.QueryKind, SearchQueryKindSelect)
	}
	if out.MatchCount != 2 {
		t.Errorf("MatchCount: got %d want 2", out.MatchCount)
	}
	if out.TotalDeferredTools != 3 {
		t.Errorf("TotalDeferredTools: got %d want 3", out.TotalDeferredTools)
	}
	if out.OccurredAt.IsZero() {
		t.Error("OccurredAt should be populated")
	}
}

// TestTelemetry_ModeDecisionEventFiresPerRequest verifies that
// ModeDecisionEvent fires for every model request with the right
// Mode and Deferred fields.
func TestTelemetry_ModeDecisionEventFiresPerRequest(t *testing.T) {
	proxy := New(Config{}) // ModeAlways (default)
	bus := core.NewEventBus()

	hide := makeDeferredTool("hide_me", "Deferred")
	type Empty struct{}
	keep := core.FuncTool[Empty]("keep_me", "Inline",
		func(_ context.Context, _ Empty) (string, error) { return "", nil })
	tools := []core.Tool{hide, keep, proxy.Tool()}

	var decisions []ModeDecisionEvent
	var mu sync.Mutex
	core.Subscribe(bus, func(e ModeDecisionEvent) {
		mu.Lock()
		decisions = append(decisions, e)
		mu.Unlock()
	})

	model := core.NewTestModel(core.TextResponse("done"))
	agent := core.NewAgent[string](model,
		core.WithTools[string](tools...),
		core.WithToolsPrepare[string](proxy.PrepareFuncFor(tools)),
		core.WithEventBus[string](bus),
	)

	if _, err := agent.Run(context.Background(), "go"); err != nil {
		t.Fatalf("agent run: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(decisions) != 1 {
		t.Fatalf("expected 1 ModeDecisionEvent, got %d", len(decisions))
	}
	d := decisions[0]
	if d.Mode != ModeAlways {
		t.Errorf("Mode: got %v want ModeAlways", d.Mode)
	}
	if !d.Deferred {
		t.Error("Deferred should be true in ModeAlways")
	}
	if d.DeferredToolCount != 1 {
		t.Errorf("DeferredToolCount: got %d want 1", d.DeferredToolCount)
	}
	if d.InlineToolCount != 2 {
		// keep_me + tool_search
		t.Errorf("InlineToolCount: got %d want 2", d.InlineToolCount)
	}
}

// TestTelemetry_ModeAutoBelowThresholdReportsNotDeferred exercises
// the auto-mode pass-through branch and confirms the event reflects
// estimated tokens and threshold.
func TestTelemetry_ModeAutoBelowThresholdReportsNotDeferred(t *testing.T) {
	proxy := New(Config{
		Mode:           ModeAuto,
		AutoTokenRatio: 0.10,
		ContextWindow:  200_000,
	})
	bus := core.NewEventBus()

	// A single short deferred tool — well under the 20k-token threshold.
	tools := []core.Tool{makeDeferredTool("tiny", "Small"), proxy.Tool()}

	var decisions []ModeDecisionEvent
	var mu sync.Mutex
	core.Subscribe(bus, func(e ModeDecisionEvent) {
		mu.Lock()
		decisions = append(decisions, e)
		mu.Unlock()
	})

	model := core.NewTestModel(core.TextResponse("done"))
	agent := core.NewAgent[string](model,
		core.WithTools[string](tools...),
		core.WithToolsPrepare[string](proxy.PrepareFuncFor(tools)),
		core.WithEventBus[string](bus),
	)
	if _, err := agent.Run(context.Background(), "go"); err != nil {
		t.Fatalf("agent run: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(decisions) != 1 {
		t.Fatalf("expected 1 ModeDecisionEvent, got %d", len(decisions))
	}
	d := decisions[0]
	if d.Mode != ModeAuto {
		t.Errorf("Mode: got %v want ModeAuto", d.Mode)
	}
	if d.Deferred {
		t.Error("Deferred should be false (below threshold)")
	}
	if d.Threshold == 0 {
		t.Error("Threshold should be populated in ModeAuto")
	}
	if d.EstimatedTokens == 0 {
		// Unlikely — at least the name+description characters should produce a nonzero estimate.
		t.Error("EstimatedTokens should be populated in ModeAuto")
	}
	if d.EstimatedTokens >= d.Threshold {
		t.Errorf("sanity: estimated %d should be below threshold %d", d.EstimatedTokens, d.Threshold)
	}
}

// TestTelemetry_SearchOutcomeHasMatchesField verifies the HasMatches
// bool is populated on the SearchOutcomeEvent, both when the query
// finds tools and when it finds nothing.
func TestTelemetry_SearchOutcomeHasMatchesField(t *testing.T) {
	proxy := New(Config{})
	bus := core.NewEventBus()

	hide := makeDeferredTool("hide", "Deferred")
	tools := []core.Tool{hide, proxy.Tool()}

	var events []SearchOutcomeEvent
	var mu sync.Mutex
	core.Subscribe(bus, func(e SearchOutcomeEvent) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})

	// Two tool_search calls: one that matches, one that doesn't.
	model := core.NewTestModel(
		core.ToolCallResponseWithID(DefaultToolName, `{"query":"select:hide"}`, "call_1"),
		core.ToolCallResponseWithID(DefaultToolName, `{"query":"select:nonexistent"}`, "call_2"),
		core.TextResponse("done"),
	)
	agent := core.NewAgent[string](model,
		core.WithTools[string](tools...),
		core.WithToolsPrepare[string](proxy.PrepareFuncFor(tools)),
		core.WithEventBus[string](bus),
	)
	if _, err := agent.Run(context.Background(), "go"); err != nil {
		t.Fatalf("agent run: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 2 {
		t.Fatalf("expected 2 SearchOutcomeEvents, got %d", len(events))
	}
	if !events[0].HasMatches {
		t.Errorf("first event should have HasMatches=true, got %+v", events[0])
	}
	if events[1].HasMatches {
		t.Errorf("second event should have HasMatches=false, got %+v", events[1])
	}
}

// TestTelemetry_ModeDecisionReasonField spot-checks that every mode
// × threshold combination emits the correct named Reason.
func TestTelemetry_ModeDecisionReasonField(t *testing.T) {
	cases := []struct {
		name       string
		cfg        Config
		tools      []core.Tool
		wantReason ModeReason
	}{
		{
			name:       "always-mode emits always_defer",
			cfg:        Config{Mode: ModeAlways},
			tools:      []core.Tool{makeDeferredTool("hide", "X")},
			wantReason: ReasonAlwaysDefer,
		},
		{
			name:       "mode-off emits mode_off",
			cfg:        Config{Mode: ModeOff},
			tools:      []core.Tool{makeDeferredTool("hide", "X")},
			wantReason: ReasonModeOff,
		},
		{
			name:       "auto-below emits auto_below_threshold",
			cfg:        Config{Mode: ModeAuto, ContextWindow: 200_000, AutoTokenRatio: 0.10},
			tools:      []core.Tool{makeDeferredTool("hide", "X")},
			wantReason: ReasonAutoBelowThreshold,
		},
		{
			name: "auto-above emits auto_above_threshold",
			cfg: Config{
				Mode:           ModeAuto,
				AutoTokenRatio: 0.0001,
				ContextWindow:  1000,
			},
			tools: []core.Tool{
				makeDeferredTool("hide_a", strings.Repeat("long description ", 50)),
				makeDeferredTool("hide_b", strings.Repeat("long description ", 50)),
			},
			wantReason: ReasonAutoAboveThreshold,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			proxy := New(tc.cfg)
			bus := core.NewEventBus()

			allTools := append(append([]core.Tool{}, tc.tools...), proxy.Tool())

			var events []ModeDecisionEvent
			var mu sync.Mutex
			core.Subscribe(bus, func(e ModeDecisionEvent) {
				mu.Lock()
				events = append(events, e)
				mu.Unlock()
			})

			model := core.NewTestModel(core.TextResponse("done"))
			agent := core.NewAgent[string](model,
				core.WithTools[string](allTools...),
				core.WithToolsPrepare[string](proxy.PrepareFuncFor(allTools)),
				core.WithEventBus[string](bus),
			)
			if _, err := agent.Run(context.Background(), "go"); err != nil {
				t.Fatalf("agent run: %v", err)
			}

			mu.Lock()
			defer mu.Unlock()
			if len(events) != 1 {
				t.Fatalf("expected 1 ModeDecisionEvent, got %d", len(events))
			}
			if events[0].Reason != tc.wantReason {
				t.Errorf("Reason: got %q want %q", events[0].Reason, tc.wantReason)
			}
		})
	}
}

// TestTelemetry_PoolChangeEventInitialFlag verifies the Initial flag
// distinguishes a first-time full announcement from an incremental
// mid-run change.
func TestTelemetry_PoolChangeEventInitialFlag(t *testing.T) {
	p := New(Config{})
	bus := core.NewEventBus()

	var events []PoolChangeEvent
	var mu sync.Mutex
	core.Subscribe(bus, func(e PoolChangeEvent) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})

	rc := &core.RunContext{EventBus: bus}

	// First call — initial announcement.
	initial := []core.Tool{makeDeferredTool("hide_A", "A")}
	if _, err := p.SystemPromptFuncFor(initial)(context.Background(), rc); err != nil {
		t.Fatalf("first call: %v", err)
	}

	// Second call — pool grew, incremental delta.
	updated := []core.Tool{
		makeDeferredTool("hide_A", "A"),
		makeDeferredTool("hide_B", "B"),
	}
	if _, err := p.SystemPromptFuncFor(updated)(context.Background(), rc); err != nil {
		t.Fatalf("second call: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 2 {
		t.Fatalf("expected 2 PoolChangeEvents, got %d", len(events))
	}
	if !events[0].Initial {
		t.Error("first announcement should have Initial=true")
	}
	if len(events[0].Added) != 1 || events[0].Added[0] != "hide_A" {
		t.Errorf("first event Added: got %v want [hide_A]", events[0].Added)
	}
	if events[1].Initial {
		t.Error("second announcement should have Initial=false")
	}
	if len(events[1].Added) != 1 || events[1].Added[0] != "hide_B" {
		t.Errorf("second event Added: got %v want [hide_B]", events[1].Added)
	}
}

// TestTelemetry_NilEventBusIsSafe proves the publishers are no-ops
// when the agent has no EventBus wired in — the common case.
func TestTelemetry_NilEventBusIsSafe(t *testing.T) {
	proxy := New(Config{})
	hide := makeDeferredTool("hide", "Deferred")
	tools := []core.Tool{hide, proxy.Tool()}

	model := core.NewTestModel(
		core.ToolCallResponseWithID(DefaultToolName, `{"query":"select:hide"}`, "call_1"),
		core.TextResponse("done"),
	)
	agent := core.NewAgent[string](model,
		core.WithTools[string](tools...),
		core.WithToolsPrepare[string](proxy.PrepareFuncFor(tools)),
		// no WithEventBus
	)
	// Must not panic or error despite no bus.
	if _, err := agent.Run(context.Background(), "go"); err != nil {
		t.Fatalf("run without event bus failed: %v", err)
	}
}

// --------------------------------------------------------------------------
// helpers
// --------------------------------------------------------------------------

// BenchmarkSearchTools_Cached measures cached keyword search latency
// against a 200-tool pool. Running the same query twice over the same
// pool should be noticeably faster on the second pass because
// parseToolName / ToLower on each candidate only runs once.
func BenchmarkSearchTools_Cached(b *testing.B) {
	pool := make([]core.Tool, 200)
	for i := range pool {
		pool[i] = makeDeferredTool(
			mockToolName(i),
			"A moderate-length tool description "+mockToolName(i),
		)
	}
	p := New(Config{})
	p.state.setPool(pool)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = searchTools("file read", pool, 5, p.state.lookupScoring)
	}
}

// BenchmarkSearchTools_Uncached is the reference point — no cache, so
// parseToolName / ToLower runs on every candidate every call.
func BenchmarkSearchTools_Uncached(b *testing.B) {
	pool := make([]core.Tool, 200)
	for i := range pool {
		pool[i] = makeDeferredTool(
			mockToolName(i),
			"A moderate-length tool description "+mockToolName(i),
		)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = searchTools("file read", pool, 5, nil)
	}
}

func mockToolName(i int) string {
	// Mix CamelCase, snake_case, and mcp__ shapes so the benchmark
	// exercises every branch of parseToolName.
	switch i % 3 {
	case 0:
		return "FileReadTool" + string(rune('0'+i%10))
	case 1:
		return "file_write_" + string(rune('a'+i%26))
	default:
		return "mcp__github__action_" + string(rune('a'+i%26))
	}
}

func namesFromDefs(defs []core.ToolDefinition) []string {
	out := make([]string, len(defs))
	for i, d := range defs {
		out[i] = d.Name
	}
	return out
}

func containsString(s []string, target string) bool {
	for _, v := range s {
		if v == target {
			return true
		}
	}
	return false
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
