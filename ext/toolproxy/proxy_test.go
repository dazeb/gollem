package toolproxy

import (
	"context"
	"encoding/json"
	"strings"
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
	got := searchTools("select:Read,Edit", tools, 5)
	want := []string{"Read", "Edit"}
	if !stringSliceEqual(got, want) {
		t.Errorf("select: got %v want %v", got, want)
	}
}

func TestSearchTools_SelectMissingIgnored(t *testing.T) {
	tools := []core.Tool{
		makeDeferredTool("Read", "Read files"),
	}
	got := searchTools("select:Read,NonExistent", tools, 5)
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

	got := searchTools("select:already_loaded", tools, 5)
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

	got := searchTools("already_loaded", tools, 5)
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
	got := searchTools("file read", tools, 5)
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
	got := searchTools("+slack send", tools, 5)
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
	got := searchTools("file", tools, 2)
	if len(got) > 2 {
		t.Errorf("max_results=2 should cap at 2: got %d results", len(got))
	}
}

func TestSearchTools_MCPPrefixScoring(t *testing.T) {
	tools := []core.Tool{
		makeDeferredTool("mcp__github__create_issue", "Create a GitHub issue"),
		makeDeferredTool("mcp__slack__send_message", "Send a Slack message"),
	}
	got := searchTools("github issue", tools, 5)
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
	got := searchTools("deploy", tools, 5)
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
// helpers
// --------------------------------------------------------------------------

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
