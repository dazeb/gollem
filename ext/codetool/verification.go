package codetool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/fugue-labs/gollem/core"
)

// VerificationCheckpoint creates a middleware and output validator pair that
// forces the agent to run verification commands (tests, builds, linting) before
// declaring completion. This is the single highest-impact harness technique for
// coding benchmarks — LangChain reported +13.7 points from harness engineering
// alone, with self-verification being the biggest contributor.
//
// The middleware tracks bash tool calls across the conversation. The output
// validator rejects the agent's output if no verification was detected, forcing
// a retry with instructions to verify.
//
// This checkpoint intentionally uses hard blockers only:
//  1. no verification run yet,
//  2. edits after the last verification run,
//  3. last verification run failed.
//
// Advisory guidance (stagnation/regression/stale-test reminders) is injected via
// middleware, but does not force extra completion turns.
//
// Usage:
//
//	mw, validator := codetool.VerificationCheckpoint("/app", timeout)
//	agent := core.NewAgent[string](model,
//	    core.WithAgentMiddleware[string](mw),
//	    core.WithOutputValidator[string](validator),
//	)
func VerificationCheckpoint(_ string, _ ...time.Duration) (core.AgentMiddleware, core.OutputValidatorFunc[string]) {
	var mu sync.Mutex
	verified := false
	lastVerifyFailed := false
	lastVerifySummary := ""
	editsSinceLastVerify := 0
	serviceCommandSeen := false
	serviceReadinessVerified := false
	invariantToolAvailable := false
	invariantToolSeen := false
	invariantStateSeen := false
	invariantItems := []invariantItem(nil)
	invariantHardUnresolved := 0
	invariantHardFail := 0
	anyMutationMade := false // whether any file-editing tool call succeeded this run
	stagnationWarned := 0    // consecutive fail level at which we last injected guidance
	staleTestWarned := false // whether we've warned about not running tests after edits
	rejectionCount := 0      // total validator rejections; capped to prevent infinite loops
	requireInvariantChecklist := envEnabled("GOLLEM_REQUIRE_INVARIANT_CHECKLIST")

	mw := core.RequestOnlyMiddleware(func(
		ctx context.Context,
		messages []core.ModelMessage,
		settings *core.ModelSettings,
		params *core.ModelRequestParameters,
		next func(context.Context, []core.ModelMessage, *core.ModelSettings, *core.ModelRequestParameters) (*core.ModelResponse, error),
	) (*core.ModelResponse, error) {
		mu.Lock()
		// Scan all messages to track verification commands and their results.
		// We rebuild the full run history each time (messages are immutable
		// so this is idempotent). This also tracks stagnation metrics.
		var pendingCallID string
		var runFailed []bool // whether each verification run failed
		var runPassed []int  // pass count per run (-1 if unavailable)
		var runSummary []string
		preVerifyVerified := false // verified state before last verification attempt
		preVerifyEdits := 0        // editsAfterLastVerify before last verification attempt
		pendingMutationCallIDs := map[string]struct{}{}
		pendingServiceReadinessCallIDs := map[string]struct{}{}
		pendingInvariantCallIDs := map[string]struct{}{}
		editsAfterLastVerify := 0 // file changes since last verification run
		serviceCommandSeenNow := false
		serviceReadinessVerifiedNow := false
		invariantToolSeenNow := false
		invariantStateSeenNow := false
		invariantItemsNow := []invariantItem(nil)
		invariantHardUnresolvedNow := 0
		invariantHardFailNow := 0
		invariantToolAvailableNow := hasTool(params, "invariants")

		for _, msg := range messages {
			if resp, ok := msg.(core.ModelResponse); ok {
				for _, part := range resp.Parts {
					if tc, ok := part.(core.ToolCallPart); ok {
						if tc.ToolName == "invariants" {
							invariantToolSeenNow = true
							if tc.ToolCallID != "" {
								pendingInvariantCallIDs[tc.ToolCallID] = struct{}{}
							}
						}
						if tc.ToolName == "bash" {
							cmd := bashCommandFromArgsJSON(tc.ArgsJSON)
							if isServiceLifecycleCommand(cmd) {
								serviceCommandSeenNow = true
								// Any subsequent start/restart should require a fresh readiness check.
								serviceReadinessVerifiedNow = false
							}
							if tc.ToolCallID != "" && isServiceReadinessCommand(cmd) {
								pendingServiceReadinessCallIDs[tc.ToolCallID] = struct{}{}
							}
						}

						isVerify := (tc.ToolName == "bash" && isVerificationCommand(tc.ArgsJSON)) ||
							(tc.ToolName == "execute_code" && isVerificationCode(tc.ArgsJSON))

						bashMutation := tc.ToolName == "bash" && isMutatingBashCommand(tc.ArgsJSON)
						if isVerify && bashMutation && !isStrongMutatingBashCommand(tc.ArgsJSON) {
							// Verification commands often redirect output to logs
							// (e.g., `pytest > /tmp/log`). Redirection alone should
							// not mark workspace dirty or cause stale-verify loops.
							bashMutation = false
						}

						isMutation := tc.ToolName == "edit" || tc.ToolName == "multi_edit" || tc.ToolName == "write" ||
							(tc.ToolName == "lsp" && isMutatingLSPCall(tc.ArgsJSON)) ||
							bashMutation

						if isVerify {
							// Save state before marking verified, so we can revert
							// if the command turns out to be "not found".
							preVerifyVerified = verified
							preVerifyEdits = editsAfterLastVerify
							verified = true
							pendingCallID = tc.ToolCallID
							editsAfterLastVerify = 0
							// Previous edits are now covered by this verification run.
							clear(pendingMutationCallIDs)
						}
						if isMutation {
							// Count edits on matching tool return, not tool call, so failed
							// edit attempts don't force unnecessary re-verification.
							if tc.ToolCallID != "" {
								pendingMutationCallIDs[tc.ToolCallID] = struct{}{}
							} else {
								// Defensive fallback for malformed call IDs.
								editsAfterLastVerify++
							}
						}
					}
				}
			}
			if req, ok := msg.(core.ModelRequest); ok {
				for _, part := range req.Parts {
					if tr, ok := part.(core.ToolReturnPart); ok {
						if pendingCallID != "" && tr.ToolCallID == pendingCallID {
							content := toolReturnContentString(tr.Content)
							// If the verification command wasn't found (e.g., pytest not
							// installed), don't count it as a failed verification run.
							// Revert to the pre-verify state so the agent isn't trapped
							// by a "last verify failed" gate it can't resolve.
							if isCommandNotFound(content) {
								verified = preVerifyVerified
								editsAfterLastVerify = preVerifyEdits
								pendingCallID = ""
							} else {
								failed, summary := verificationResultFailed(content)
								p, _, countsOK := extractTestCounts(content)
								if !countsOK {
									// Unknown pass count must not be treated as 0, otherwise
									// regression logic can fabricate false "1 -> 0" regressions.
									p = -1
								}
								runFailed = append(runFailed, failed)
								runPassed = append(runPassed, p)
								runSummary = append(runSummary, summary)
								pendingCallID = ""
							}
						}
						if _, ok := pendingMutationCallIDs[tr.ToolCallID]; ok {
							content := toolReturnContentString(tr.Content)
							if mutationToolReturnSucceeded(tr.ToolName, content) {
								editsAfterLastVerify++
								anyMutationMade = true
							}
							delete(pendingMutationCallIDs, tr.ToolCallID)
						}
						if _, ok := pendingServiceReadinessCallIDs[tr.ToolCallID]; ok {
							content := toolReturnContentString(tr.Content)
							if isServiceReadinessResultSuccessful(content) {
								serviceReadinessVerifiedNow = true
							}
							delete(pendingServiceReadinessCallIDs, tr.ToolCallID)
						}
						if _, ok := pendingInvariantCallIDs[tr.ToolCallID]; ok {
							content := toolReturnContentString(tr.Content)
							if gateState, stateOK := extractInvariantGateState(content); stateOK {
								invariantStateSeenNow = true
								if gateState.hasItems {
									invariantItemsNow = cloneInvariantItems(gateState.items)
								}
								if gateState.hasCounts {
									invariantHardUnresolvedNow = gateState.hardUnresolved
									invariantHardFailNow = gateState.hardFail
								}
							}
							delete(pendingInvariantCallIDs, tr.ToolCallID)
						}
					}
				}
			}
		}

		// Update latest result for the validator.
		if len(runFailed) > 0 {
			lastVerifyFailed = runFailed[len(runFailed)-1]
			lastVerifySummary = runSummary[len(runSummary)-1]
		}
		editsSinceLastVerify = editsAfterLastVerify
		serviceCommandSeen = serviceCommandSeenNow
		serviceReadinessVerified = serviceReadinessVerifiedNow
		invariantToolAvailable = invariantToolAvailableNow
		invariantToolSeen = invariantToolSeenNow
		invariantStateSeen = invariantStateSeenNow
		invariantItems = cloneInvariantItems(invariantItemsNow)
		invariantHardUnresolved = invariantHardUnresolvedNow
		invariantHardFail = invariantHardFailNow

		// Compute stagnation: count consecutive failing runs from the end
		// that aren't showing improvement in pass counts.
		consecutiveFails := 0
		for i := len(runFailed) - 1; i >= 0; i-- {
			if !runFailed[i] {
				break
			}
			consecutiveFails++
		}

		// Check if pass counts are improving across the failing streak.
		// If the agent went from 2 passed → 5 passed, that's progress
		// even though tests are still failing overall.
		isImproving := false
		if consecutiveFails >= 2 {
			streakStart := len(runPassed) - consecutiveFails
			firstPassed := runPassed[streakStart]
			lastPassedCnt := runPassed[len(runPassed)-1]
			if firstPassed >= 0 && lastPassedCnt > firstPassed {
				isImproving = true
			}
		}

		// Detect regression: pass count decreased between the last two runs.
		// This means the agent's last change BROKE something that was working.
		// Distinct from stagnation — regression needs a "revert" nudge.
		isRegression := false
		if len(runPassed) >= 2 {
			prev := runPassed[len(runPassed)-2]
			curr := runPassed[len(runPassed)-1]
			if prev >= 0 && curr >= 0 && curr < prev {
				isRegression = true
			}
		}

		sw := stagnationWarned
		stw := staleTestWarned
		ealv := editsAfterLastVerify
		mu.Unlock()

		// Inject regression warning when the agent's last change broke tests.
		// This takes priority over stagnation guidance since it's more actionable.
		if isRegression && len(runPassed) >= 2 {
			prev := runPassed[len(runPassed)-2]
			curr := runPassed[len(runPassed)-1]
			guidance := fmt.Sprintf(
				"REGRESSION DETECTED: Your last change BROKE tests — passed went from %d → %d.\n"+
					"Your most recent edit caused previously passing tests to FAIL.\n"+
					"1. UNDO your last change (revert the file or restore the working version)\n"+
					"2. Re-run tests to confirm the revert restores the pass count to %d\n"+
					"3. Then try a DIFFERENT fix that doesn't break existing tests\n"+
					"NEVER fix one test by breaking another. All tests must pass simultaneously.",
				prev, curr, prev)
			fmt.Fprintf(os.Stderr, "[gollem] verification: regression detected — %d → %d passed\n", prev, curr)
			messages = injectUserPromptIntoLastRequest(messages, guidance)
		} else if consecutiveFails >= 2 && consecutiveFails > sw && !isImproving {
			// Inject stagnation guidance when the agent isn't making progress.
			// Only inject when the stagnation level increases past a threshold
			// we haven't warned about yet, and skip if improving.
			mu.Lock()
			stagnationWarned = consecutiveFails
			mu.Unlock()

			guidance := stagnationGuidance(consecutiveFails, runPassed, runSummary)
			fmt.Fprintf(os.Stderr, "[gollem] verification: stagnation detected — %d consecutive failing runs\n", consecutiveFails)
			messages = injectUserPromptIntoLastRequest(messages, guidance)
		}

		// Detect "stale test" — agent making many file edits without re-running
		// tests. This catches the failure mode where the agent runs tests once,
		// then enters a prolonged "edit only" phase without verifying changes.
		const staleTestThreshold = 6
		if verified && !stw && ealv >= staleTestThreshold {
			mu.Lock()
			staleTestWarned = true
			mu.Unlock()
			guidance := fmt.Sprintf("TESTING REMINDER: You've made %d file changes since your last test run. "+
				"Run tests now to verify your changes work. Iterative test→fix→test cycles "+
				"catch issues early and are more effective than making many changes before testing.",
				ealv)
			fmt.Fprintf(os.Stderr, "[gollem] verification: stale test — %d edits since last verify\n", ealv)
			messages = injectUserPromptIntoLastRequest(messages, guidance)
		}
		// Reset stale test warning when a new verification resets the counter.
		if stw && ealv < staleTestThreshold {
			mu.Lock()
			staleTestWarned = false
			mu.Unlock()
		}

		return next(ctx, messages, settings, params)
	})

	const maxRejections = 3

	// reject is a helper that increments the rejection counter and returns a retry error.
	reject := func(output, gate, message string) (string, error) {
		mu.Lock()
		rejectionCount++
		rc := rejectionCount
		mu.Unlock()
		fmt.Fprintf(os.Stderr, "[gollem] verification: gate %q rejected completion (%d/%d)\n", gate, rc, maxRejections)
		return output, &core.ModelRetryError{Message: message}
	}

	validator := func(_ context.Context, _ *core.RunContext, output string) (string, error) {
		mu.Lock()
		v := verified
		mutated := anyMutationMade
		lvf := lastVerifyFailed
		lvs := lastVerifySummary
		eslv := editsSinceLastVerify
		svcSeen := serviceCommandSeen
		svcReady := serviceReadinessVerified
		invAvailable := invariantToolAvailable
		invSeen := invariantToolSeen
		invStateSeen := invariantStateSeen
		invItems := cloneInvariantItems(invariantItems)
		invHardUnresolved := invariantHardUnresolved
		invHardFail := invariantHardFail
		rc := rejectionCount
		mu.Unlock()

		// Safety cap: after maxRejections total rejections, allow completion
		// to prevent infinite loops from any cause (pattern matching gaps,
		// unfixable test failures, etc.).
		if rc >= maxRejections {
			fmt.Fprintf(os.Stderr, "[gollem] verification: rejection cap reached (%d), allowing completion\n", maxRejections)
			return output, nil
		}

		if !v && mutated {
			return reject(output, "not-verified",
				"STOP. You MUST verify your changes before completing the task. "+
					"Run the relevant test suite (e.g., `go test ./...`, `pytest`, `npm test`) "+
					"and/or build command (e.g., `go build ./...`, `make`, `cargo build`) to "+
					"confirm your changes are correct. If a test tool is not installed, install "+
					"it first (e.g., `pip install pytest`). Do NOT declare completion without "+
					"evidence that your solution works.")
		}

		// If files were edited after the last verification run, force a fresh
		// verification before completion. This prevents stale "tests passed"
		// state after subsequent code changes.
		if eslv > 0 {
			return reject(output, "edits-after-verify", fmt.Sprintf(
				"STOP. You made %d file edit(s) since your last verification run. "+
					"Run tests/build again now and ensure they pass before declaring completion.",
				eslv))
		}

		// If the last verification run failed, force the agent to fix and
		// re-verify. Never allow completion with failing tests.
		if lvf {
			msg := "STOP. Your last verification run FAILED"
			if lvs != "" {
				msg += ": " + lvs
			}
			msg += "\n" + failureGuidance(lvs)
			msg += "Do NOT declare completion with failing tests."
			return reject(output, "last-verify-failed", msg)
		}
		if svcSeen && !svcReady {
			return reject(output, "service-not-ready",
				"STOP. This task used service lifecycle commands, but no successful readiness proof was recorded. "+
					"Before completion, prove service liveness the way the verifier will (e.g., port check with `ss -tlnp`/`nc -z` "+
					"and a real protocol request such as curl/grpc call).")
		}
		if requireInvariantChecklist && invAvailable {
			if !invSeen {
				return reject(output, "invariants-not-run",
					"STOP. You must run the `invariants` tool before completion. "+
						"Run `invariants` command `extract`, then verify/update statuses with evidence.")
			}
			if !invStateSeen {
				return reject(output, "invariants-no-state",
					"STOP. Invariant state is missing. Run the `invariants` tool (`get`, `summary`, or `update`) after your latest invariant changes so the checklist can be verified.")
			}
			if invHardFail > 0 || invHardUnresolved > 0 {
				msg := fmt.Sprintf(
					"STOP. Hard invariants are not all PASS (hard_fail=%d, hard_unresolved=%d). "+
						"Fix remaining violations, update invariant statuses with evidence, and re-run verification.",
					invHardFail, invHardUnresolved,
				)
				if details := formatPendingHardInvariants(invItems, 4); details != "" {
					msg += "\n" + details
				}
				return reject(output, "invariants-failing", msg)
			}
		}

		return output, nil
	}

	return mw, validator
}

// isVerificationCommand checks whether a bash tool call's ArgsJSON contains a
// command that looks like a test, build, or lint verification step.
func isVerificationCommand(argsJSON string) bool {
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return false
	}
	return isVerificationString(strings.ToLower(args.Command))
}

// isVerificationCode checks whether an execute_code tool call's ArgsJSON
// contains code that looks like verification (running tests, checking output,
// comparing results). This handles the case where the agent uses execute_code
// instead of bash for verification.
func isVerificationCode(argsJSON string) bool {
	var args struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return false
	}
	code := strings.ToLower(args.Code)

	// If the code shells out via bash(), treat as verification.
	// execute_code wraps tool calls as Python functions, e.g.:
	//   bash(command="python /app/test_outputs.py")
	//   bash(command="pytest")
	if strings.Contains(code, "bash(") {
		return true
	}

	// Check for verification-like code patterns.
	codeVerifyPatterns := []string{
		"assert ", "assert(", // Python assertions
		"assertEqual", "assertTrue", // unittest assertions
		"test_output", "test_result", "run_test",
		"verify(", "validate(",
		"expected", "== expected",
		"diff(", "compare(",
		"open(", // reading output files to check them
	}
	for _, p := range codeVerifyPatterns {
		if strings.Contains(code, p) {
			return true
		}
	}
	return false
}

func bashCommandFromArgsJSON(argsJSON string) string {
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ""
	}
	return args.Command
}

// isCommandNotFound checks whether verification output indicates the command
// binary wasn't found (e.g., pytest not installed). These should not count as
// failed verification runs since verification was never actually performed.
func isCommandNotFound(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "command not found") ||
		strings.Contains(lower, "no such file or directory") ||
		(strings.Contains(lower, "no module named") && strings.Contains(output, "[exit code:"))
}

func isServiceLifecycleCommand(cmd string) bool {
	lower := strings.ToLower(strings.TrimSpace(cmd))
	if lower == "" {
		return false
	}

	if strings.Contains(lower, "nohup ") {
		return true
	}
	if strings.Contains(lower, "systemctl ") &&
		(strings.Contains(lower, " start ") || strings.Contains(lower, " restart ") ||
			strings.Contains(lower, " enable ") || strings.Contains(lower, " stop ")) {
		return true
	}
	if strings.Contains(lower, "service ") &&
		(strings.Contains(lower, " start") || strings.Contains(lower, " restart") || strings.Contains(lower, " stop")) {
		return true
	}

	lifecyclePatterns := []string{
		"python3 /app/server.py",
		"python /app/server.py",
		"uvicorn ",
		"gunicorn ",
		"nginx",
		"redis-server",
		"mongod",
		"sshd",
	}
	for _, p := range lifecyclePatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

func isServiceReadinessCommand(cmd string) bool {
	lower := strings.ToLower(strings.TrimSpace(cmd))
	if lower == "" {
		return false
	}

	readinessPatterns := []string{
		"ss -tln",
		"ss -ltn",
		"netstat -tln",
		"lsof -i",
		"nc -z",
		"curl localhost",
		"curl 127.0.0.1",
		"curl http://localhost",
		"curl http://127.0.0.1",
		"grpcurl ",
		"connect_ex(",
		"grpc.insecure_channel(",
	}
	for _, p := range readinessPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

func isServiceReadinessResultSuccessful(content string) bool {
	lower := strings.ToLower(strings.TrimSpace(content))
	if lower == "" {
		return false
	}
	if strings.HasPrefix(lower, "error:") {
		return false
	}
	if strings.Contains(lower, "[timed out after") || strings.Contains(lower, "[exit code:") {
		return false
	}
	failPatterns := []string{
		"connection refused",
		"failed to connect",
		"not listening",
		"statuscode.unavailable",
		"no route to host",
		"name or service not known",
	}
	for _, p := range failPatterns {
		if strings.Contains(lower, p) {
			return false
		}
	}
	return true
}

// isMutatingLSPCall returns true when an lsp tool call is expected to apply
// file edits (rename, or code_action with action_index). Listing code actions
// without action_index is read-only and must not mark the workspace dirty.
func isMutatingLSPCall(argsJSON string) bool {
	var args struct {
		Method      string `json:"method"`
		ActionIndex *int   `json:"action_index,omitempty"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return false
	}

	switch strings.ToLower(strings.TrimSpace(args.Method)) {
	case "rename":
		return true
	case "code_action":
		return args.ActionIndex != nil
	default:
		return false
	}
}

// invariantGateState captures the most recent checklist view parsed from an
// invariants tool result. Some results include full items, while others may only
// expose aggregate counts.
type invariantGateState struct {
	items          []invariantItem
	hardUnresolved int
	hardFail       int
	hasItems       bool
	hasCounts      bool
}

func cloneInvariantItems(items []invariantItem) []invariantItem {
	if items == nil {
		return nil
	}
	cp := make([]invariantItem, len(items))
	copy(cp, items)
	return cp
}

// extractInvariantGateCounts parses the JSON output of the invariants tool to
// read hard_unresolved and hard_fail counts. Returns ok=false (silently) if the
// content isn't the expected shape. Prefer extractInvariantGateState when item-
// level detail is needed.
//
//nolint:unused // Retained as a convenience wrapper; tests and external callers may use it.
func extractInvariantGateCounts(content string) (hardUnresolved int, hardFail int, ok bool) {
	gateState, ok := extractInvariantGateState(content)
	if !ok || !gateState.hasCounts {
		return 0, 0, false
	}
	return gateState.hardUnresolved, gateState.hardFail, true
}

func extractInvariantGateState(content string) (invariantGateState, bool) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &payload); err != nil {
		return invariantGateState{}, false
	}

	state := invariantGateState{}
	if rawItems, ok := payload["items"]; ok {
		b, err := json.Marshal(rawItems)
		if err == nil {
			var items []invariantItem
			if err := json.Unmarshal(b, &items); err == nil {
				state.items = normalizeInvariantGateItems(items)
				state.hasItems = true
				state.hardUnresolved, state.hardFail = countPendingHardInvariants(state.items)
				state.hasCounts = true
			}
		}
	}

	unresolvedAny, unresolvedOK := payload["hard_unresolved"]
	failAny, failOK := payload["hard_fail"]
	if unresolvedOK && failOK {
		unresolved, unresolvedConv := anyToInt(unresolvedAny)
		fail, failConv := anyToInt(failAny)
		if unresolvedConv && failConv {
			state.hardUnresolved = unresolved
			state.hardFail = fail
			state.hasCounts = true
		}
	}

	if !state.hasItems && !state.hasCounts {
		return invariantGateState{}, false
	}
	return state, true
}

func normalizeInvariantGateItems(items []invariantItem) []invariantItem {
	normalized := make([]invariantItem, 0, len(items))
	for _, item := range items {
		normalized = append(normalized, invariantItem{
			ID:          strings.TrimSpace(item.ID),
			Description: strings.TrimSpace(item.Description),
			Kind:        normalizeInvariantKind(item.Kind),
			Status:      normalizeInvariantStatusOrDefault(item.Status),
			Evidence:    strings.TrimSpace(item.Evidence),
		})
	}
	return normalized
}

func countPendingHardInvariants(items []invariantItem) (hardUnresolved int, hardFail int) {
	for _, item := range items {
		if normalizeInvariantKind(item.Kind) != "hard" {
			continue
		}
		switch normalizeInvariantStatusOrDefault(item.Status) {
		case "pass":
			continue
		case "fail":
			hardFail++
		default:
			hardUnresolved++
		}
	}
	return hardUnresolved, hardFail
}

func pendingHardInvariantItems(items []invariantItem) []invariantItem {
	pending := make([]invariantItem, 0, len(items))
	for _, item := range items {
		if normalizeInvariantKind(item.Kind) != "hard" {
			continue
		}
		if normalizeInvariantStatusOrDefault(item.Status) == "pass" {
			continue
		}
		pending = append(pending, item)
	}
	return pending
}

func formatPendingHardInvariants(items []invariantItem, maxShow int) string {
	pending := pendingHardInvariantItems(items)
	if len(pending) == 0 {
		return ""
	}
	if maxShow <= 0 {
		maxShow = 3
	}
	if maxShow > len(pending) {
		maxShow = len(pending)
	}

	var b strings.Builder
	b.WriteString("Pending hard invariants:\n")
	for i := range maxShow {
		item := pending[i]
		id := strings.TrimSpace(item.ID)
		if id == "" {
			id = fmt.Sprintf("I%d", i+1)
		}
		status := normalizeInvariantStatusOrDefault(item.Status)
		description := strings.TrimSpace(item.Description)
		if description == "" {
			description = "(missing description)"
		}
		fmt.Fprintf(&b, "- %s [hard/%s] %q", id, status, description)
		if evidence := strings.TrimSpace(item.Evidence); evidence != "" {
			fmt.Fprintf(&b, " — evidence: %s", evidence)
		} else {
			b.WriteString(" — no evidence")
		}
		b.WriteByte('\n')
	}
	if remaining := len(pending) - maxShow; remaining > 0 {
		fmt.Fprintf(&b, "- +%d more pending hard invariants", remaining)
	} else {
		return strings.TrimRight(b.String(), "\n")
	}
	return b.String()
}

func anyToInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	default:
		return 0, false
	}
}

// isMutatingBashCommand returns true when a bash tool call likely mutates the
// filesystem (file edits, writes, copies, deletes, etc.). This keeps stale
// verification detection aligned with real post-test code changes.
func isMutatingBashCommand(argsJSON string) bool {
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return false
	}
	return isMutatingBashString(args.Command)
}

func isStrongMutatingBashCommand(argsJSON string) bool {
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return false
	}
	return hasStrongBashMutationString(args.Command)
}

func isMutatingBashString(command string) bool {
	if strings.TrimSpace(command) == "" {
		return false
	}

	if hasStrongBashMutationString(command) {
		return true
	}

	lower := strings.ToLower(command)

	// tee can write files; treat as mutating to avoid stale verification.
	if strings.Contains(lower, "tee ") {
		return true
	}

	// Redirections are file writes except for common fd redirects (2>, 1>, &>).
	for i := range len(command) {
		if command[i] != '>' {
			continue
		}
		if i > 0 {
			prev := command[i-1]
			if (prev >= '0' && prev <= '9') || prev == '&' {
				continue
			}
		}
		if i+1 < len(command) && command[i+1] == '&' {
			continue
		}
		return true
	}

	return false
}

func hasStrongBashMutationString(command string) bool {
	lower := strings.ToLower(command)

	// Common file-mutation operations.
	patterns := []string{
		"sed -i", "perl -i", "perl -pi",
		"rm ", "mv ", "cp ", "mkdir ", "rmdir ", "touch ", "ln ",
		"truncate ", "dd ", "patch ", "git apply",
	}
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}

	// install (coreutils) writes files; ignore package-manager installs.
	if strings.Contains(lower, "install ") &&
		!strings.Contains(lower, "pip install") && !strings.Contains(lower, "npm install") &&
		!strings.Contains(lower, "apt install") && !strings.Contains(lower, "apt-get install") {
		return true
	}

	return false
}

// mutationToolReturnSucceeded returns whether a mutation call appears to have
// succeeded. We only count successful mutations as "edits since verify".
func mutationToolReturnSucceeded(toolName, content string) bool {
	lower := strings.ToLower(strings.TrimSpace(content))
	if strings.HasPrefix(lower, "error:") {
		return false
	}

	if toolName == "bash" {
		if strings.Contains(lower, "[timed out after") || strings.Contains(lower, "[exit code:") {
			return false
		}
	}

	if toolName == "lsp" {
		if strings.Contains(lower, "no edits were produced") ||
			strings.Contains(lower, "has no workspace edit") ||
			strings.Contains(lower, "produced no edits") ||
			strings.Contains(lower, "rename not supported") ||
			strings.Contains(lower, "no renamable symbol found") ||
			strings.Contains(lower, "cannot rename at this location") {
			return false
		}
	}

	return true
}

// nonVerificationCommands is a blocklist of commands that are NEVER used to
// verify correctness. Everything NOT in this list is treated as potential
// verification. This inverted approach (blocklist instead of allowlist) is
// robust against unknown languages, frameworks, and task-specific scripts —
// the list is small, stable, and doesn't grow with every new tool.
var nonVerificationCommands = map[string]struct{}{
	// File I/O (read only).
	"cat": {}, "head": {}, "tail": {}, "less": {}, "more": {}, "bat": {},
	"hexdump": {}, "od": {}, "strings": {},
	// Directory listing.
	"ls": {}, "dir": {}, "tree": {}, "find": {},
	// Navigation.
	"cd": {}, "pwd": {}, "pushd": {}, "popd": {},
	// Output.
	"echo": {}, "printf": {},
	// File management.
	"mkdir": {}, "touch": {}, "chmod": {}, "chown": {}, "cp": {}, "mv": {},
	"rm": {}, "rmdir": {}, "ln": {},
	// Environment.
	"export": {}, "source": {}, "unset": {}, "alias": {}, "set": {},
	"printenv": {},
	// Editors.
	"vim": {}, "nano": {}, "vi": {}, "emacs": {}, "code": {}, "ed": {},
	// System info.
	"which": {}, "type": {}, "whoami": {}, "hostname": {},
	"uname": {}, "date": {}, "uptime": {}, "id": {}, "groups": {}, "locale": {},
	// Control flow.
	"true": {}, "false": {}, "sleep": {}, "wait": {}, "read": {},
	// Process management.
	"kill": {}, "pkill": {}, "killall": {},
	// Version control (never runs tests).
	"git": {}, "svn": {}, "hg": {},
	// Install-only package managers.
	"pip": {}, "pip3": {}, "apt": {}, "apt-get": {}, "dpkg": {},
	"brew": {}, "conda": {}, "snap": {}, "yum": {}, "dnf": {},
	// Archiving.
	"tar": {}, "zip": {}, "unzip": {}, "gzip": {}, "gunzip": {},
	"bzip2": {}, "xz": {},
	// Shell utilities.
	"history": {}, "clear": {}, "reset": {}, "man": {}, "info": {},
	"help": {}, "tput": {},
	// File transfer.
	"scp": {}, "rsync": {}, "sftp": {}, "ftp": {},
	// Text processing (editing/extraction, not verification).
	"sed": {}, "awk": {},
}

// commandSplitter splits shell command chains on &&, ||, ;, |, and newlines.
var commandSplitter = regexp.MustCompile(`\s*(?:&&|\|\||[;\n|])\s*`)

// commandModifiers are binaries that wrap another command and should be
// skipped when extracting the "real" binary from a sub-command.
var commandModifiers = map[string]struct{}{
	"sudo": {}, "env": {}, "nice": {}, "timeout": {}, "time": {},
	"nohup": {}, "strace": {}, "ltrace": {}, "exec": {},
	"unbuffer": {}, "script": {}, "ionice": {}, "taskset": {},
	"chrt": {}, "numactl": {}, "command": {},
}

// splitShellCommands splits a shell command string into sub-commands by
// common separators (&&, ||, ;, |, newlines). This is a heuristic — it
// doesn't handle quoted strings or subshells, but is good enough for
// verification classification.
func splitShellCommands(cmd string) []string {
	parts := commandSplitter.Split(cmd, -1)
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// extractFirstBinary extracts the first "real" binary name from a shell
// sub-command, skipping env-var assignments (FOO=bar), command modifiers
// (sudo, env, timeout, etc.), flags (-x), and pure numbers (timeout values).
// Returns the base name (filepath.Base) for blocklist lookup.
func extractFirstBinary(subcmd string) string {
	// Strip leading subshell parens.
	subcmd = strings.TrimLeft(subcmd, "( ")
	tokens := strings.Fields(subcmd)

	i := 0
	for i < len(tokens) {
		tok := tokens[i]

		// Skip env-var assignments: FOO=bar (but not -flag or /path/cmd=arg).
		if strings.Contains(tok, "=") && len(tok) > 0 &&
			tok[0] != '-' && tok[0] != '/' && tok[0] != '.' {
			// Check first char is a letter or underscore (valid env var start).
			r := rune(tok[0])
			if unicode.IsLetter(r) || r == '_' {
				i++
				continue
			}
		}

		base := filepath.Base(tok)

		// Skip command modifiers and their arguments.
		if _, isMod := commandModifiers[base]; isMod {
			i++
			// Skip flags and numeric args that belong to the modifier
			// (e.g., timeout -k 5 30 cmd, sudo -u root cmd).
			for i < len(tokens) {
				next := tokens[i]
				if strings.HasPrefix(next, "-") {
					i++
					// If flag likely takes a value (e.g., -u root, -k 5),
					// skip the next token too if it doesn't look like a flag.
					if i < len(tokens) && !strings.HasPrefix(tokens[i], "-") {
						// Check if it's a value (not another command).
						val := tokens[i]
						if isNumeric(val) || (len(val) > 0 && unicode.IsLower(rune(val[0])) && !strings.Contains(val, "/")) {
							i++
						}
					}
					continue
				}
				if isNumeric(next) {
					i++
					continue
				}
				// Also skip env-var assignments after modifiers (env FOO=bar cmd).
				if strings.Contains(next, "=") && len(next) > 0 {
					r := rune(next[0])
					if unicode.IsLetter(r) || r == '_' {
						i++
						continue
					}
				}
				break
			}
			continue
		}

		return base
	}
	return ""
}

// isVerificationString checks whether a command string looks like a
// verification step (test, build, lint, constraint check). It uses a
// blocklist approach: a short, stable list of commands that are definitively
// NOT verification. Everything not on the blocklist is treated as potential
// verification. This is robust against unknown languages and frameworks.
func isVerificationString(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return false
	}

	// Help/discovery commands should not be treated as verification runs.
	// They don't execute tests and can create false failing-verification loops.
	if strings.Contains(cmd, "pytest --help") ||
		strings.Contains(cmd, "pytest -h") ||
		strings.Contains(cmd, "python -m pytest --help") ||
		strings.Contains(cmd, "python3 -m pytest --help") {
		return false
	}

	// Split into sub-commands and check each one's primary binary.
	// If ANY sub-command is not in the blocklist, the command is verification.
	subcmds := splitShellCommands(cmd)
	for _, sub := range subcmds {
		binary := extractFirstBinary(sub)
		if binary == "" {
			continue
		}
		if _, blocked := nonVerificationCommands[binary]; !blocked {
			return true
		}
	}
	return false
}

// checkExpectedOutputsExist checks whether expected output files/directories
// actually exist. Returns a warning string if missing outputs are detected,
// or empty string if everything looks fine.
//
// expectedOutputs is pre-computed by the caller (via detectExpectedOutputs)
// to avoid scanning test files multiple times.
// autoCleanupIntermediates removes known build artifacts that can interfere
// with test verification (many tests use os.listdir/ls to check directory
// contents). Returns the count of items removed. Only removes safe targets:
// __pycache__ dirs, *.pyc files, *.o object files, and a.out.
func autoCleanupIntermediates(workDir string) int {
	cleaned := 0

	// Cache directories to remove recursively.
	// These are common intermediates that can cause "extra files" test failures
	// when tests check directory contents with os.listdir/ls.
	cacheDirs := map[string]bool{
		"__pycache__":        true,
		".pytest_cache":      true,
		".mypy_cache":        true,
		".ruff_cache":        true,
		".tox":               true,
		".eggs":              true,
		"nimcache":           true, // Nim compilation cache
		".nimcache":          true, // Nim compilation cache (dot variant)
		".zig-cache":         true, // Zig build cache
		"zig-out":            true, // Zig build output
		".dub":               true, // D language package cache
		".ipynb_checkpoints": true, // Jupyter notebook checkpoints
	}

	// Intermediate file extensions to remove: .pyc (Python), .class (Java),
	// .hi (Haskell), .beam (Erlang/Elixir). These cause "extra files" failures.
	intermediateExts := map[string]bool{
		".pyc":   true,
		".class": true,
		".hi":    true,
		".beam":  true,
	}

	// WalkDir is faster than Walk: avoids Stat on every entry.
	// We only need file info for extension-based cleanup on matching files.
	if err := filepath.WalkDir(workDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		name := d.Name()
		if d.IsDir() {
			if cacheDirs[name] {
				if os.RemoveAll(path) == nil {
					cleaned++
				}
				return filepath.SkipDir
			}
			// *.egg-info directories (Python packaging artifacts).
			if strings.HasSuffix(name, ".egg-info") {
				if os.RemoveAll(path) == nil {
					cleaned++
				}
				return filepath.SkipDir
			}
			// node_modules/.cache (build tool caches).
			if name == ".cache" && filepath.Base(filepath.Dir(path)) == "node_modules" {
				if os.RemoveAll(path) == nil {
					cleaned++
				}
				return filepath.SkipDir
			}
			return nil
		}
		// Intermediate files by extension.
		if intermediateExts[filepath.Ext(name)] {
			if os.Remove(path) == nil {
				cleaned++
			}
		}
		return nil
	}); err != nil {
		return cleaned
	}

	// Remove *.o and a.out in the workDir root only (not recursively —
	// subdirectories may contain intentional object files).
	entries, err := os.ReadDir(workDir)
	if err != nil {
		return cleaned
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "a.out" || strings.HasSuffix(name, ".o") {
			path := filepath.Join(workDir, name)
			if os.Remove(path) == nil {
				cleaned++
			}
		}
	}
	return cleaned
}

func checkExpectedOutputsExist(workDir string, expectedOutputs []string) string {
	// Check for common output directories that should be populated.
	outputDirs := []struct {
		path string
		name string
	}{
		{filepath.Join(workDir, "output_data"), "output_data/"},
		{"/app/task_file/output_data", "/app/task_file/output_data/"},
		{filepath.Join(workDir, "output"), "output/"},
	}
	for _, od := range outputDirs {
		info, err := os.Stat(od.path)
		if err != nil || !info.IsDir() {
			continue
		}
		entries, err := os.ReadDir(od.path)
		if err != nil {
			continue
		}
		// Count non-hidden files.
		fileCount := 0
		for _, e := range entries {
			if !strings.HasPrefix(e.Name(), ".") {
				fileCount++
			}
		}
		if fileCount == 0 {
			return fmt.Sprintf("6. WARNING: %s directory exists but is EMPTY! You likely need to create output files in it.\n", od.name)
		}
	}

	// Check for expected solution files (common deliverable names).
	solutionFiles := []string{
		"solution.py", "solution.js", "solution.ts", "solution.go",
		"solution.rs", "solution.c", "solution.cpp", "solution.java",
		"solution.rb", "solution.sh",
	}
	for _, sf := range solutionFiles {
		for _, dir := range []string{workDir, "/app"} {
			path := filepath.Join(dir, sf)
			if info, err := os.Stat(path); err == nil && info.Size() == 0 {
				return fmt.Sprintf("6. WARNING: %s exists but is EMPTY (0 bytes)! Write your solution to it.\n", sf)
			}
		}
	}

	// Check expected output paths exist and are non-empty.
	// expectedOutputs is pre-computed by the caller to avoid scanning test files twice.
	var missingOutputs []string
	for _, o := range expectedOutputs {
		// Resolve the path.
		path := o
		if !filepath.IsAbs(path) {
			path = filepath.Join(workDir, path)
		}
		info, err := os.Stat(path)
		if err != nil {
			// Try /app as alternative base.
			altPath := filepath.Join("/app", o)
			if _, altErr := os.Stat(altPath); altErr != nil {
				missingOutputs = append(missingOutputs, o)
			}
		} else if info.Size() == 0 {
			missingOutputs = append(missingOutputs, o+" (EMPTY)")
		}
	}
	if len(missingOutputs) > 0 {
		if len(missingOutputs) > 5 {
			missingOutputs = missingOutputs[:5]
		}
		return fmt.Sprintf("6. WARNING: Expected output files are MISSING or EMPTY: %s\n"+
			"   Tests reference these files — create them before declaring completion.\n",
			strings.Join(missingOutputs, ", "))
	}

	return ""
}

// validateOutputFormats programmatically checks output files for common format
// issues that cause test failures: BOM markers, Windows line endings, invalid
// JSON, and missing executable bits. Returns a warning string or empty.
func validateOutputFormats(workDir string, expectedOutputs []string) string {
	// Gather output files from multiple sources for comprehensive checking.
	seen := make(map[string]bool)
	var outputFiles []string

	addFile := func(path string) {
		if !seen[path] {
			seen[path] = true
			outputFiles = append(outputFiles, path)
		}
	}

	// Source 1: files detected from test scripts (pre-computed by caller).
	for _, o := range expectedOutputs {
		addFile(o)
	}

	// Source 2: files in output directories.
	for _, dirName := range []string{"output_data", "output"} {
		for _, base := range []string{workDir, "/app", "/app/task_file"} {
			dir := filepath.Join(base, dirName)
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if !e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
					addFile(filepath.Join(dirName, e.Name()))
				}
			}
		}
	}

	// Source 3: common output file patterns in workDir.
	for _, pattern := range []string{"output.*", "result.*", "results.*", "answer.*"} {
		matches, _ := filepath.Glob(filepath.Join(workDir, pattern))
		for _, m := range matches {
			info, err := os.Stat(m)
			if err != nil || info.IsDir() {
				continue
			}
			rel, _ := filepath.Rel(workDir, m)
			if rel != "" {
				addFile(rel)
			}
		}
	}

	formatHints := detectOutputFormat(workDir)

	// Determine expected formats.
	expectBinary := false
	for _, h := range formatHints {
		if strings.HasPrefix(h, "EXECUTABLE:") {
			expectBinary = true
		}
	}

	var issues []string

	// Check each output file for format issues.
	for _, o := range outputFiles {
		path := o
		if !filepath.IsAbs(path) {
			path = filepath.Join(workDir, path)
		}
		info, err := os.Stat(path)
		if err != nil {
			// Try /app as alternative.
			altPath := filepath.Join("/app", o)
			if altInfo, altErr := os.Stat(altPath); altErr == nil {
				path = altPath
				info = altInfo
			} else {
				continue // file doesn't exist, handled by checkExpectedOutputsExist
			}
		}

		// Only check text/data files under 1MB.
		if info.Size() > 1024*1024 {
			continue
		}

		data, err := os.ReadFile(path)
		if err != nil || len(data) == 0 {
			continue
		}

		// Check for UTF-8 BOM (0xEF 0xBB 0xBF).
		if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
			issues = append(issues, fmt.Sprintf("%s has UTF-8 BOM marker — remove it: sed -i '1s/^\\xEF\\xBB\\xBF//' %s", o, path))
		}

		// Check for Windows line endings (\r\n).
		if bytes.Contains(data, []byte("\r\n")) {
			issues = append(issues, fmt.Sprintf("%s has Windows line endings (\\r\\n) — convert: sed -i 's/\\r$//' %s", o, path))
		}

		// Check trailing newline for text files. Most test frameworks
		// expect text output to end with a newline. Missing trailing
		// newline is one of the top 3 format-related failures.
		if len(data) > 0 && !isBinaryLike(data) {
			if data[len(data)-1] != '\n' {
				issues = append(issues, fmt.Sprintf("%s is missing a trailing newline — most tests expect one. Add with: printf '\\n' >> %s", o, path))
			}
		}

		// Validate JSON/JSONL files unconditionally — invalid JSON is one of the
		// most common output format failures. Don't gate on expectJSON since
		// any .json file should be valid JSON regardless of how tests check it.
		if strings.HasSuffix(o, ".json") {
			if !json.Valid(bytes.TrimSpace(data)) {
				issues = append(issues, o+" is not valid JSON — check syntax (mismatched braces, trailing commas, unquoted keys)")
			}
		} else if strings.HasSuffix(o, ".jsonl") {
			// Check first few lines of JSONL to catch structural errors early.
			lines := bytes.SplitN(data, []byte("\n"), 6) // check up to 5 lines
			for idx, line := range lines {
				line = bytes.TrimSpace(line)
				if len(line) == 0 {
					continue
				}
				if !json.Valid(line) {
					issues = append(issues, fmt.Sprintf("%s: line %d is not valid JSON", o, idx+1))
					break
				}
			}
		}

		// Validate CSV structure: consistent column count across rows.
		// Mismatched column counts (e.g., header has N columns but data has
		// N-1 or N+1) are a frequent cause of test failures.
		if strings.HasSuffix(o, ".csv") || strings.HasSuffix(o, ".tsv") {
			delim := byte(',')
			if strings.HasSuffix(o, ".tsv") {
				delim = byte('\t')
			}
			csvLines := bytes.SplitN(data, []byte("\n"), 12) // check first 10 data lines
			headerCols := -1
			mismatchLine := -1
			for idx, line := range csvLines {
				line = bytes.TrimRight(line, "\r")
				if len(line) == 0 {
					continue
				}
				cols := bytes.Count(line, []byte{delim}) + 1
				if headerCols < 0 {
					headerCols = cols
				} else if cols != headerCols && mismatchLine < 0 {
					mismatchLine = idx + 1
				}
			}
			if mismatchLine > 0 {
				issues = append(issues, fmt.Sprintf("%s: inconsistent column count — header has %d columns but line %d differs. Check delimiters and quoting.", o, headerCols, mismatchLine))
			}
		}

		if len(issues) >= 3 {
			break
		}
	}

	// Check executable bit if binary expected.
	if expectBinary {
		binaryName := detectExpectedBinaryName(nil)
		if binaryName == "" {
			// Try common names.
			for _, name := range []string{"solution", "program", "main", "a.out"} {
				for _, dir := range []string{workDir, "/app"} {
					p := filepath.Join(dir, name)
					if info, err := os.Stat(p); err == nil && !info.IsDir() {
						if info.Mode()&0o111 == 0 {
							issues = append(issues, fmt.Sprintf("%s exists but is NOT executable — run: chmod +x %s", name, p))
						}
						break
					}
				}
			}
		}
	}

	if len(issues) == 0 {
		return ""
	}
	result := "7. FORMAT ISSUES found in your output files:\n"
	var resultSb1098 strings.Builder
	for _, issue := range issues {
		resultSb1098.WriteString("   - " + issue + "\n")
	}
	result += resultSb1098.String()
	result += "   Fix these before declaring completion — they WILL cause test failures.\n"
	return result
}

// failureGuidance returns targeted recovery advice based on the type of
// verification failure. More specific than generic "fix the failures".
func failureGuidance(summary string) string {
	lower := strings.ToLower(summary)
	switch {
	case strings.Contains(lower, "timed out") || strings.Contains(lower, "timeout"):
		return "Your solution is TOO SLOW. Profile with `time` and optimize the hot path. " +
			"Consider: more efficient algorithm, caching, avoiding redundant computation. " +
			"If using Python: use numpy/vectorized ops, dict/set for lookups, generators for large data.\n"
	case strings.Contains(lower, "compilation") || strings.Contains(lower, "compile") ||
		strings.Contains(lower, "syntax error") || strings.Contains(lower, "undefined"):
		return "Fix the COMPILATION ERRORS first — read the error messages for exact file:line locations.\n"
	case strings.Contains(lower, "expected") && strings.Contains(lower, "got"):
		return "Output MISMATCH — compare expected vs actual values character-by-character. " +
			"Check: trailing newlines, whitespace, numeric precision, encoding. " +
			"Use `xxd <your_output> | head -5` and `xxd <expected_output> | head -5` to compare bytes.\n"
	case strings.Contains(lower, "not found") || strings.Contains(lower, "no such file"):
		return "MISSING FILE — check that you created all required output files in the right location. " +
			"Use `ls -la` to verify file existence and paths.\n"
	case strings.Contains(lower, "permission denied"):
		return "PERMISSION ERROR — try: chmod +x <script> or chmod 644 <file>. " +
			"For services: check if the process needs root or a specific user.\n"
	case strings.Contains(lower, "connection refused") || strings.Contains(lower, "connection reset"):
		return "SERVICE NOT RUNNING — check if the server/daemon started successfully. " +
			"Use: ss -tlnp to check listening ports, service <name> status for service state.\n"
	case strings.Contains(lower, "import") && (strings.Contains(lower, "error") || strings.Contains(lower, "module")):
		return "IMPORT ERROR — a required module is missing. " +
			"Install with: pip install --break-system-packages <module>. " +
			"If it's a local module, check PYTHONPATH or run from the correct directory.\n"
	case strings.Contains(lower, "segfault") || strings.Contains(lower, "segmentation fault") ||
		strings.Contains(lower, "signal: segmentation") || strings.Contains(lower, "sigsegv") ||
		strings.Contains(lower, "signal 11") || strings.Contains(lower, "exit code: 139"):
		return "SEGMENTATION FAULT — your code is accessing invalid memory. " +
			"Common causes: array/buffer out of bounds, null/dangling pointer dereference, use after free. " +
			"Debug with: valgrind ./program (if available), or add bounds checks to array accesses. " +
			"In C/C++: check all pointer dereferences and array indices.\n"
	case strings.Contains(lower, "killed") || strings.Contains(lower, "out of memory") ||
		strings.Contains(lower, "oom") || strings.Contains(lower, "signal 9") ||
		strings.Contains(lower, "exit code: 137") || strings.Contains(lower, "cannot allocate memory"):
		return "OUT OF MEMORY — your solution uses too much RAM and was killed by the OS. " +
			"Reduce memory usage: process data in streaming/chunked fashion instead of loading all into memory, " +
			"use generators/iterators, free large data structures when no longer needed. " +
			"If using Python: avoid building large lists — use generators. If using C/C++: free() after use.\n"
	case strings.Contains(lower, "stack overflow") || strings.Contains(lower, "maximum recursion depth") ||
		strings.Contains(lower, "recursionerror") || strings.Contains(lower, "stack level too deep"):
		return "STACK OVERFLOW — your code has infinite or very deep recursion. " +
			"Convert recursive algorithms to iterative (use an explicit stack/queue). " +
			"If the recursion depth is legitimate, increase the limit: " +
			"Python: sys.setrecursionlimit(N), C/C++: ulimit -s unlimited.\n"
	case strings.Contains(lower, "floating point exception") || strings.Contains(lower, "sigfpe") ||
		strings.Contains(lower, "division by zero") || strings.Contains(lower, "divide by zero") ||
		strings.Contains(lower, "exit code: 136"):
		return "FLOATING POINT EXCEPTION — your code has division by zero or integer overflow. " +
			"Add guards before every division: check that divisors are non-zero. " +
			"For integer overflow: use larger types (int64/long) or check before multiply.\n"
	case strings.Contains(lower, "deadlock") || strings.Contains(lower, "all goroutines are asleep") ||
		strings.Contains(lower, "fatal error: concurrent") || strings.Contains(lower, "data race"):
		return "CONCURRENCY BUG — your code has a deadlock or race condition. " +
			"Check: lock ordering (always acquire locks in the same order), channel operations " +
			"(ensure sends have matching receives), and shared state access (use mutexes/atomics). " +
			"In Go: run with `go test -race` to detect races. In Python: check threading.Lock usage.\n"
	case strings.Contains(lower, "unicodedecodeerror") || strings.Contains(lower, "codec") ||
		strings.Contains(lower, "charmap") || strings.Contains(lower, "invalid utf") ||
		strings.Contains(lower, "can't decode") || strings.Contains(lower, "encoding error"):
		return "ENCODING ERROR — your code can't decode/encode text properly. " +
			"Use explicit encoding: open(file, encoding='utf-8'), or handle bytes mode. " +
			"In Python: try `errors='replace'` or `errors='ignore'` as a fallback. " +
			"Check if input files use a non-UTF-8 encoding (latin-1, cp1252).\n"
	case strings.Contains(lower, "wrong answer") || strings.Contains(lower, "incorrect") ||
		strings.Contains(lower, "mismatch") || strings.Contains(lower, "does not match"):
		return "WRONG OUTPUT — your solution produces incorrect results. " +
			"Compare your output vs expected output character-by-character. " +
			"Common issues: off-by-one errors, integer vs float, sorting order, rounding precision. " +
			"Re-read the problem statement for edge cases you may have missed.\n"
	case strings.Contains(lower, "assert"):
		return "ASSERTION FAILURE — read the test code to understand exactly what's expected. " +
			"Fix one failure at a time, starting with the first.\n"
	default:
		return "Fix the failures and re-run verification before completing. "
	}
}

// stagnationGuidance returns progressively stronger guidance based on how many
// consecutive verification runs have failed without improvement.
func stagnationGuidance(consecutiveFails int, runPassed []int, runSummary []string) string {
	// Build a brief run history for context.
	var history string
	streakStart := len(runPassed) - consecutiveFails
	if streakStart < 0 {
		streakStart = 0
	}
	var historySb1196 strings.Builder
	for i := streakStart; i < len(runPassed); i++ {
		run := i - streakStart + 1
		summary := ""
		if i < len(runSummary) {
			summary = runSummary[i]
		}
		if runPassed[i] >= 0 {
			fmt.Fprintf(&historySb1196, "  Run %d: %s\n", run, summary)
		} else if summary != "" {
			history += fmt.Sprintf("  Run %d: %s\n", run, summary)
		}
	}
	history += historySb1196.String()

	// Detect if the same error is repeating across runs. When two consecutive
	// summaries contain the same failure detail, the agent's edits aren't
	// addressing the root cause — a stronger nudge is needed.
	sameError := false
	if len(runSummary) >= 2 {
		prev := runSummary[len(runSummary)-2]
		curr := runSummary[len(runSummary)-1]
		if prev != "" && curr != "" && prev == curr {
			sameError = true
		}
	}

	sameErrorHint := ""
	if sameError {
		sameErrorHint = "NOTE: The EXACT SAME error appeared in the last two runs. " +
			"Your edits are NOT fixing the root cause. " +
			"Stop, re-read the error, and fix the ACTUAL problem — not what you think the problem is.\n"
	}

	switch consecutiveFails {
	case 2:
		msg := "VERIFICATION STAGNATION: Tests have failed 2 times in a row.\n"
		if history != "" {
			msg += "Run history:\n" + history
		}
		msg += sameErrorHint
		msg += "Re-read the FULL error output from the last test run — you may be " +
			"misunderstanding the requirement or fixing the wrong thing.\n" +
			"Before making more changes, re-read the test file to confirm what's actually expected."
		return msg

	case 3:
		msg := "STAGNATION WARNING: Tests have failed 3 times in a row without improvement.\n"
		if history != "" {
			msg += "Run history:\n" + history
		}
		msg += sameErrorHint
		msg += "Your current approach may be fundamentally wrong. Do these NOW:\n" +
			"1. Re-read the ORIGINAL task description from scratch\n" +
			"2. Re-read the test file assertions character by character\n" +
			"3. Compare your output with expected output using diff\n" +
			"4. Consider if you're solving the WRONG PROBLEM entirely"
		return msg

	default: // 4+
		msg := fmt.Sprintf("CRITICAL STAGNATION: Tests have failed %d times in a row. "+
			"STOP making incremental fixes — they are NOT working.\n", consecutiveFails)
		if history != "" {
			msg += "Run history:\n" + history
		}
		msg += sameErrorHint
		msg += "You MUST try a FUNDAMENTALLY DIFFERENT approach:\n" +
			"- If output format is wrong: dump the expected output in hex (xxd) and compare byte-by-byte\n" +
			"- If algorithm is wrong: switch to a simpler brute-force approach, then optimize\n" +
			"- If compilation keeps failing: rewrite the solution file from scratch\n" +
			"- If you keep getting the same test failure: the test might expect something you haven't considered — " +
			"read the test code line by line, including imports and helper functions"
		return msg
	}
}

// isBinaryLike checks if file data appears to be binary (not text).
// Used to skip trailing newline checks on binary/image files.
func isBinaryLike(data []byte) bool {
	// Check first 512 bytes for NUL characters (binary indicator).
	checkLen := len(data)
	if checkLen > 512 {
		checkLen = 512
	}
	for _, b := range data[:checkLen] {
		if b == 0 {
			return true
		}
	}
	return false
}

// toolReturnContentString extracts a string from a ToolReturnPart's Content field.
func toolReturnContentString(content any) string {
	if s, ok := content.(string); ok {
		return s
	}
	b, err := json.Marshal(content)
	if err != nil {
		return fmt.Sprintf("%v", content)
	}
	return string(b)
}

// verificationResultFailed checks whether a verification command's output
// indicates failure (non-zero exit code, test failures, build errors, timeouts).
// Returns (failed, summary) where summary describes what went wrong.
func verificationResultFailed(output string) (bool, string) {
	hasNonZeroExit := strings.Contains(output, "[exit code:") &&
		!strings.Contains(output, "[exit code: 0]")
	hasTimeout := strings.Contains(output, "[timed out after")

	if !hasNonZeroExit && !hasTimeout {
		// Some frameworks report failures even with exit code 0.
		if _, f, ok := extractTestCounts(output); ok && f > 0 {
			summary := testResultSummary(output)
			if summary == "" {
				summary = fmt.Sprintf("%d test(s) failed", f)
			}
			return true, summary
		}
		return false, ""
	}

	// Try to extract a specific failure summary.
	if summary := testResultSummary(output); summary != "" {
		return true, summary
	}
	// Try extracting test counts directly (works with shorter output).
	if p, f, ok := extractTestCounts(output); ok && f > 0 {
		return true, fmt.Sprintf("%d passed, %d failed", p, f)
	}
	if summary := compilationErrorSummary(output, 1); summary != "" {
		return true, summary
	}
	if detail := firstFailureDetail(output); detail != "" {
		return true, detail
	}
	if hasTimeout {
		return true, "verification command timed out"
	}
	return true, "verification command exited with non-zero status"
}
