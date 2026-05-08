//nolint:gosec,perfsprint // Trace CLI paths and flag indexes are local operator-supplied inputs.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fugue-labs/gollem/core"
	temporalext "github.com/fugue-labs/gollem/ext/temporal"
	traceutil "github.com/fugue-labs/gollem/ext/trace"
	"github.com/fugue-labs/gollem/ext/tui"
	"go.temporal.io/sdk/client"
)

var traceForkRunExec = func(args []string, stdin io.Reader, stdout, stderr io.Writer, env []string) error {
	cmd := exec.CommandContext(context.Background(), os.Args[0], args...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = env
	return cmd.Run()
}

var traceQueryTemporalArtifact = queryTemporalTraceArtifact
var traceViewArtifact = tui.TraceView
var traceCompareArtifacts = tui.TraceCompareView

func runTraceCommand() {
	if len(os.Args) < 3 {
		printTraceUsage()
		os.Exit(1)
	}

	if err := dispatchTraceCommand(os.Args[2], os.Args[3:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		printTraceUsage()
		os.Exit(1)
	}
}

func dispatchTraceCommand(cmd string, args []string) error {
	switch cmd {
	case "export":
		return runTraceExport(args)
	case "inspect", "summarize":
		return runTraceInspect(args)
	case "view":
		return runTraceView(args)
	case "replay":
		return runTraceReplay(args)
	case "fork":
		return runTraceFork(args)
	case "diff":
		return runTraceDiff(args)
	case "regress":
		return runTraceRegress(args)
	case "sleepy":
		return runTraceSleepy(args)
	case "validate":
		return runTraceValidate(args)
	case "redact":
		return runTraceRedact(args)
	case "compact":
		return runTraceCompact(args)
	case "--help", "-h", "help":
		printTraceUsage()
		return nil
	default:
		return fmt.Errorf("unknown trace command: %s", cmd)
	}
}

func runTraceExport(args []string) error {
	opts, err := parseTraceExportArgs(args)
	if err != nil {
		return err
	}
	var artifact *traceutil.Artifact
	if opts.temporal {
		artifact, err = traceQueryTemporalArtifact(opts)
		if err != nil {
			return err
		}
	} else {
		artifact, err = exportLocalTraceArtifact(opts)
		if err != nil {
			return err
		}
	}
	if opts.out == "" {
		opts.out = "-"
	}
	if err := traceutil.WriteFile(opts.out, artifact); err != nil {
		return fmt.Errorf("write trace artifact: %w", err)
	}
	return nil
}

func runTraceInspect(args []string) error {
	input, limit, err := parseTraceInspectArgs(args)
	if err != nil {
		return err
	}
	artifact, err := traceutil.ReadFile(input)
	if err != nil {
		return fmt.Errorf("read trace: %w", err)
	}
	return traceutil.Inspect(os.Stdout, artifact, traceutil.InspectOptions{EventsLimit: limit})
}

func runTraceView(args []string) error {
	paths, err := parseTraceViewArgs(args)
	if err != nil {
		return err
	}
	artifact, err := traceutil.ReadFile(paths[0])
	if err != nil {
		return fmt.Errorf("read trace: %w", err)
	}
	if len(paths) == 2 {
		variant, readErr := traceutil.ReadFile(paths[1])
		if readErr != nil {
			return fmt.Errorf("read variant trace: %w", readErr)
		}
		return traceCompareArtifacts(artifact, variant)
	}
	return traceViewArtifact(artifact)
}

func runTraceReplay(args []string) error {
	input, mode, err := parseTraceReplayArgs(args)
	if err != nil {
		return err
	}
	if !traceutil.SupportedReplayMode(mode) {
		return fmt.Errorf("unsupported replay mode %q (supported: inspect, strict, simulated, fork, live-reexec)", mode)
	}
	artifact, err := traceutil.ReadFile(input)
	if err != nil {
		return fmt.Errorf("read trace: %w", err)
	}
	return traceutil.ReplayWithOptions(os.Stdout, artifact, traceutil.ReplayOptions{Mode: mode})
}

func runTraceFork(args []string) error {
	input, out, opts, runOpts, err := parseTraceForkArgs(args)
	if err != nil {
		return err
	}
	artifact, err := traceutil.ReadFile(input)
	if err != nil {
		return fmt.Errorf("read trace: %w", err)
	}
	snap, record, err := traceutil.ForkSnapshot(artifact, opts)
	if err != nil {
		return err
	}
	if runOpts.Continue {
		return continueTraceFork(out, snap, record, runOpts)
	}
	if out == "" {
		out = "-"
	}
	if err := traceutil.WriteSnapshotFile(out, snap); err != nil {
		return fmt.Errorf("write fork snapshot: %w", err)
	}
	if out != "-" {
		fmt.Fprintf(os.Stderr, "gollem: fork snapshot written to %s (from %s step=%d, run_id=%s)\n", out, record.ID, record.Step, snap.RunID)
	}
	return nil
}

type traceForkRunOptions struct {
	Continue    bool
	SnapshotOut string
	Provider    string
	WorkDir     string
	Timeout     string
	ExtraArgs   []string
}

func continueTraceFork(out string, snap *core.RunSnapshot, record traceutil.SnapshotRecord, runOpts traceForkRunOptions) error {
	if strings.TrimSpace(out) == "" || out == "-" {
		return fmt.Errorf("trace fork --continue requires --out <forked.trace.json>")
	}
	snapshotPath := strings.TrimSpace(runOpts.SnapshotOut)
	cleanupSnapshot := false
	if snapshotPath == "" {
		tmp, err := os.CreateTemp("", "gollem-fork-*.snapshot.json")
		if err != nil {
			return fmt.Errorf("create temporary fork snapshot: %w", err)
		}
		snapshotPath = tmp.Name()
		if err := tmp.Close(); err != nil {
			return fmt.Errorf("close temporary fork snapshot: %w", err)
		}
		cleanupSnapshot = true
	}
	if cleanupSnapshot {
		defer func() { _ = os.Remove(snapshotPath) }()
	}
	if err := traceutil.WriteSnapshotFile(snapshotPath, snap); err != nil {
		return fmt.Errorf("write fork snapshot: %w", err)
	}
	args := []string{"run", "--resume-snapshot", snapshotPath, "--trace-out", out}
	if runOpts.Provider != "" {
		args = append(args, "--provider", runOpts.Provider)
	}
	if runOpts.WorkDir != "" {
		args = append(args, "--workdir", runOpts.WorkDir)
	}
	if runOpts.Timeout != "" {
		args = append(args, "--timeout", runOpts.Timeout)
	}
	args = append(args, runOpts.ExtraArgs...)
	if err := traceForkRunExec(args, os.Stdin, os.Stdout, os.Stderr, os.Environ()); err != nil {
		return fmt.Errorf("continue fork run from %s step=%d: %w", record.ID, record.Step, err)
	}
	fmt.Fprintf(os.Stderr, "gollem: fork trace written to %s (from %s step=%d, run_id=%s)\n", out, record.ID, record.Step, snap.RunID)
	return nil
}

func runTraceDiff(args []string) error {
	format := "text"
	var paths []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--format":
			if i+1 >= len(args) {
				return fmt.Errorf("--format requires a value")
			}
			format = strings.TrimSpace(args[i+1])
			i++
		case "--help", "-h":
			printTraceUsage()
			os.Exit(0)
		default:
			if strings.HasPrefix(args[i], "-") {
				return fmt.Errorf("unknown diff argument %q", args[i])
			}
			paths = append(paths, args[i])
		}
	}
	if len(paths) != 2 {
		return fmt.Errorf("diff requires baseline and variant trace paths")
	}
	baseline, err := traceutil.ReadFile(paths[0])
	if err != nil {
		return fmt.Errorf("read baseline trace: %w", err)
	}
	variant, err := traceutil.ReadFile(paths[1])
	if err != nil {
		return fmt.Errorf("read variant trace: %w", err)
	}
	diff := traceutil.Diff(baseline, variant)
	switch format {
	case "", "text":
		return traceutil.WriteDiff(os.Stdout, diff)
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(diff)
	default:
		return fmt.Errorf("unsupported diff format %q (supported: text, json)", format)
	}
}

func runTraceRegress(args []string) error {
	format := "text"
	var opts traceutil.RegressionOptions
	var paths []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--format":
			if i+1 >= len(args) {
				return fmt.Errorf("--format requires a value")
			}
			format = strings.TrimSpace(args[i+1])
			i++
		case "--max-cost-delta":
			if i+1 >= len(args) {
				return fmt.Errorf("--max-cost-delta requires a value")
			}
			value, parseErr := strconv.ParseFloat(args[i+1], 64)
			if parseErr != nil {
				return fmt.Errorf("invalid --max-cost-delta value %q", args[i+1])
			}
			opts.MaxCostDelta = &value
			i++
		case "--max-token-delta":
			if i+1 >= len(args) {
				return fmt.Errorf("--max-token-delta requires a value")
			}
			value, parseErr := strconv.Atoi(args[i+1])
			if parseErr != nil {
				return fmt.Errorf("invalid --max-token-delta value %q", args[i+1])
			}
			opts.MaxTokenDelta = &value
			i++
		case "--require-status":
			if i+1 >= len(args) {
				return fmt.Errorf("--require-status requires a value")
			}
			opts.RequireStatus = strings.TrimSpace(args[i+1])
			i++
		case "--help", "-h":
			printTraceUsage()
			os.Exit(0)
		default:
			if strings.HasPrefix(args[i], "-") {
				return fmt.Errorf("unknown regress argument %q", args[i])
			}
			paths = append(paths, args[i])
		}
	}
	if len(paths) < 2 {
		return fmt.Errorf("regress requires a baseline trace and at least one variant trace")
	}
	baseline, err := traceutil.ReadFile(paths[0])
	if err != nil {
		return fmt.Errorf("read baseline trace: %w", err)
	}
	variants := make([]*traceutil.Artifact, 0, len(paths)-1)
	for _, path := range paths[1:] {
		variant, readErr := traceutil.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read variant trace %s: %w", path, readErr)
		}
		variants = append(variants, variant)
	}
	report := traceutil.Regress(baseline, variants, opts)
	switch format {
	case "", "text":
		return traceutil.WriteRegressionReport(os.Stdout, report)
	case "json":
		return traceutil.WriteRegressionReportJSON(os.Stdout, report)
	default:
		return fmt.Errorf("unsupported regress format %q (supported: text, json)", format)
	}
}

func runTraceSleepy(args []string) error {
	out := "-"
	var paths []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--out", "-o":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a value", args[i])
			}
			out = args[i+1]
			i++
		case "--help", "-h":
			printTraceUsage()
			os.Exit(0)
		default:
			if strings.HasPrefix(args[i], "-") && args[i] != "-" {
				return fmt.Errorf("unknown sleepy argument %q", args[i])
			}
			paths = append(paths, args[i])
		}
	}
	if len(paths) < 2 {
		return fmt.Errorf("sleepy requires a baseline trace and at least one candidate trace")
	}
	baseline, err := traceutil.ReadFile(paths[0])
	if err != nil {
		return fmt.Errorf("read baseline trace: %w", err)
	}
	candidates := make([]*traceutil.Artifact, 0, len(paths)-1)
	for _, path := range paths[1:] {
		candidate, readErr := traceutil.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read candidate trace %s: %w", path, readErr)
		}
		candidates = append(candidates, candidate)
	}
	evidence, err := traceutil.BuildSleepyEvidence(baseline, candidates)
	if err != nil {
		return err
	}
	return traceutil.WriteSleepyEvidenceFile(out, evidence)
}

func runTraceValidate(args []string) error {
	input, err := parseSingleTracePath(args)
	if err != nil {
		return err
	}
	artifact, err := traceutil.ReadFile(input)
	if err != nil {
		return fmt.Errorf("read trace: %w", err)
	}
	if err := traceutil.ValidateArtifact(artifact); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "ok: %s (%d events)\n", traceutil.SchemaVersion, len(artifact.Events))
	return nil
}

func runTraceRedact(args []string) error {
	input, out, opts, err := parseTraceRedactArgs(args)
	if err != nil {
		return err
	}
	artifact, err := traceutil.ReadFile(input)
	if err != nil {
		return fmt.Errorf("read trace: %w", err)
	}
	redacted, err := traceutil.Redact(artifact, opts)
	if err != nil {
		return err
	}
	if out == "" {
		out = "-"
	}
	return traceutil.WriteFile(out, redacted)
}

func runTraceCompact(args []string) error {
	input, out, opts, err := parseTraceCompactArgs(args)
	if err != nil {
		return err
	}
	artifact, err := traceutil.ReadFile(input)
	if err != nil {
		return fmt.Errorf("read trace: %w", err)
	}
	compacted, err := traceutil.Compact(artifact, opts)
	if err != nil {
		return err
	}
	if out == "" {
		out = "-"
	}
	return traceutil.WriteFile(out, compacted)
}

func parseTraceForkArgs(args []string) (input string, out string, opts traceutil.ForkOptions, runOpts traceForkRunOptions, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--out", "-o":
			if i+1 >= len(args) {
				return "", "", opts, runOpts, fmt.Errorf("%s requires a value", args[i])
			}
			out = args[i+1]
			i++
		case "--continue":
			runOpts.Continue = true
		case "--snapshot-out":
			if i+1 >= len(args) {
				return "", "", opts, runOpts, fmt.Errorf("--snapshot-out requires a value")
			}
			runOpts.SnapshotOut = args[i+1]
			i++
		case "--provider":
			if i+1 >= len(args) {
				return "", "", opts, runOpts, fmt.Errorf("--provider requires a value")
			}
			runOpts.Provider = args[i+1]
			i++
		case "--workdir":
			if i+1 >= len(args) {
				return "", "", opts, runOpts, fmt.Errorf("--workdir requires a value")
			}
			runOpts.WorkDir = args[i+1]
			i++
		case "--timeout":
			if i+1 >= len(args) {
				return "", "", opts, runOpts, fmt.Errorf("--timeout requires a value")
			}
			runOpts.Timeout = args[i+1]
			i++
		case "--run-arg":
			if i+1 >= len(args) {
				return "", "", opts, runOpts, fmt.Errorf("--run-arg requires a value")
			}
			runOpts.ExtraArgs = append(runOpts.ExtraArgs, args[i+1])
			i++
		case "--from-step":
			if i+1 >= len(args) {
				return "", "", opts, runOpts, fmt.Errorf("--from-step requires a value")
			}
			if _, scanErr := fmt.Sscanf(args[i+1], "%d", &opts.FromStep); scanErr != nil || opts.FromStep < 0 {
				return "", "", opts, runOpts, fmt.Errorf("invalid --from-step value %q", args[i+1])
			}
			i++
		case "--from-event":
			if i+1 >= len(args) {
				return "", "", opts, runOpts, fmt.Errorf("--from-event requires a value")
			}
			opts.FromEventID = args[i+1]
			i++
		case "--from-checkpoint", "--from-snapshot":
			if i+1 >= len(args) {
				return "", "", opts, runOpts, fmt.Errorf("%s requires a value", args[i])
			}
			opts.FromCheckpoint = args[i+1]
			i++
		case "--from-kind":
			if i+1 >= len(args) {
				return "", "", opts, runOpts, fmt.Errorf("--from-kind requires a value")
			}
			opts.FromKind = args[i+1]
			i++
		case "--run-id":
			if i+1 >= len(args) {
				return "", "", opts, runOpts, fmt.Errorf("--run-id requires a value")
			}
			opts.NewRunID = args[i+1]
			i++
		case "--prompt":
			if i+1 >= len(args) {
				return "", "", opts, runOpts, fmt.Errorf("--prompt requires a value")
			}
			opts.Prompt = args[i+1]
			i++
		case "--system-prompt":
			if i+1 >= len(args) {
				return "", "", opts, runOpts, fmt.Errorf("--system-prompt requires a value")
			}
			prompt, err := readTraceTextOrFile(args[i+1])
			if err != nil {
				return "", "", opts, runOpts, fmt.Errorf("read --system-prompt: %w", err)
			}
			opts.SystemPrompt = prompt
			i++
		case "--planner-prompt":
			if i+1 >= len(args) {
				return "", "", opts, runOpts, fmt.Errorf("--planner-prompt requires a value")
			}
			prompt, err := readTraceTextOrFile(args[i+1])
			if err != nil {
				return "", "", opts, runOpts, fmt.Errorf("read --planner-prompt: %w", err)
			}
			opts.PlannerPrompt = prompt
			i++
		case "--append-user":
			if i+1 >= len(args) {
				return "", "", opts, runOpts, fmt.Errorf("--append-user requires a value")
			}
			opts.AppendUser = args[i+1]
			i++
		case "--model":
			if i+1 >= len(args) {
				return "", "", opts, runOpts, fmt.Errorf("--model requires a value")
			}
			opts.Model = args[i+1]
			i++
		case "--topology":
			if i+1 >= len(args) {
				return "", "", opts, runOpts, fmt.Errorf("--topology requires a value")
			}
			opts.Topology = args[i+1]
			i++
		case "--middleware":
			if i+1 >= len(args) {
				return "", "", opts, runOpts, fmt.Errorf("--middleware requires a value")
			}
			opts.Middleware = args[i+1]
			i++
		case "--tool-policy":
			if i+1 >= len(args) {
				return "", "", opts, runOpts, fmt.Errorf("--tool-policy requires a value")
			}
			opts.ToolPolicy = args[i+1]
			i++
		case "--evaluator":
			if i+1 >= len(args) {
				return "", "", opts, runOpts, fmt.Errorf("--evaluator requires a value")
			}
			opts.Evaluator = args[i+1]
			i++
		case "--memory-edit":
			if i+1 >= len(args) {
				return "", "", opts, runOpts, fmt.Errorf("--memory-edit requires key=value")
			}
			opts.MemoryEdits = append(opts.MemoryEdits, args[i+1])
			i++
		case "--help", "-h":
			printTraceUsage()
			os.Exit(0)
		default:
			if strings.HasPrefix(args[i], "-") && args[i] != "-" {
				return "", "", opts, runOpts, fmt.Errorf("unknown fork argument %q", args[i])
			}
			if input != "" {
				return "", "", opts, runOpts, fmt.Errorf("fork accepts exactly one trace path")
			}
			input = args[i]
		}
	}
	if input == "" {
		return "", "", opts, runOpts, fmt.Errorf("fork requires a trace path")
	}
	return input, out, opts, runOpts, nil
}

func parseTraceRedactArgs(args []string) (input string, out string, opts traceutil.RedactOptions, err error) {
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--out" || arg == "-o":
			if i+1 >= len(args) {
				return "", "", opts, fmt.Errorf("%s requires a value", args[i])
			}
			out = args[i+1]
			i++
		case arg == "--key":
			if i+1 >= len(args) {
				return "", "", opts, fmt.Errorf("--key requires a value")
			}
			opts.Keys = append(opts.Keys, args[i+1])
			i++
		case arg == "--pattern":
			if i+1 >= len(args) {
				return "", "", opts, fmt.Errorf("--pattern requires a value")
			}
			opts.Patterns = append(opts.Patterns, args[i+1])
			i++
		case arg == "--replacement":
			if i+1 >= len(args) {
				return "", "", opts, fmt.Errorf("--replacement requires a value")
			}
			opts.Replacement = args[i+1]
			i++
		case arg == "--drop-trace":
			value, consumed, parseErr := parseOptionalBoolFlag(args, i)
			if parseErr != nil {
				return "", "", opts, fmt.Errorf("--drop-trace: %w", parseErr)
			}
			opts.DropTrace = value
			i += consumed
		case strings.HasPrefix(arg, "--drop-trace="):
			value, parseErr := parseBoolString(strings.TrimPrefix(arg, "--drop-trace="))
			if parseErr != nil {
				return "", "", opts, fmt.Errorf("--drop-trace: %w", parseErr)
			}
			opts.DropTrace = value
		case arg == "--help" || arg == "-h":
			printTraceUsage()
			os.Exit(0)
		default:
			if strings.HasPrefix(arg, "-") && arg != "-" {
				return "", "", opts, fmt.Errorf("unknown redact argument %q", arg)
			}
			if input != "" {
				return "", "", opts, fmt.Errorf("redact accepts exactly one trace path")
			}
			input = arg
		}
	}
	if input == "" {
		return "", "", opts, fmt.Errorf("redact requires a trace path")
	}
	return input, out, opts, nil
}

func parseTraceCompactArgs(args []string) (input string, out string, opts traceutil.CompactOptions, err error) {
	opts = traceutil.CompactOptions{
		DropTrace:         true,
		EventPayloadLimit: 4096,
		KeepSnapshots:     -1,
	}
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--out" || arg == "-o":
			if i+1 >= len(args) {
				return "", "", opts, fmt.Errorf("%s requires a value", args[i])
			}
			out = args[i+1]
			i++
		case arg == "--payload-limit":
			if i+1 >= len(args) {
				return "", "", opts, fmt.Errorf("--payload-limit requires a value")
			}
			if _, scanErr := fmt.Sscanf(args[i+1], "%d", &opts.EventPayloadLimit); scanErr != nil || opts.EventPayloadLimit < 0 {
				return "", "", opts, fmt.Errorf("invalid --payload-limit value %q", args[i+1])
			}
			i++
		case arg == "--keep-snapshots":
			if i+1 >= len(args) {
				return "", "", opts, fmt.Errorf("--keep-snapshots requires a value")
			}
			if _, scanErr := fmt.Sscanf(args[i+1], "%d", &opts.KeepSnapshots); scanErr != nil || opts.KeepSnapshots < 0 {
				return "", "", opts, fmt.Errorf("invalid --keep-snapshots value %q", args[i+1])
			}
			i++
		case arg == "--keep-trace":
			opts.DropTrace = false
		case arg == "--drop-trace":
			value, consumed, parseErr := parseOptionalBoolFlag(args, i)
			if parseErr != nil {
				return "", "", opts, fmt.Errorf("--drop-trace: %w", parseErr)
			}
			opts.DropTrace = value
			i += consumed
		case strings.HasPrefix(arg, "--drop-trace="):
			value, parseErr := parseBoolString(strings.TrimPrefix(arg, "--drop-trace="))
			if parseErr != nil {
				return "", "", opts, fmt.Errorf("--drop-trace: %w", parseErr)
			}
			opts.DropTrace = value
		case arg == "--help" || arg == "-h":
			printTraceUsage()
			os.Exit(0)
		default:
			if strings.HasPrefix(arg, "-") && arg != "-" {
				return "", "", opts, fmt.Errorf("unknown compact argument %q", arg)
			}
			if input != "" {
				return "", "", opts, fmt.Errorf("compact accepts exactly one trace path")
			}
			input = arg
		}
	}
	if input == "" {
		return "", "", opts, fmt.Errorf("compact requires a trace path")
	}
	return input, out, opts, nil
}

type traceExportOptions struct {
	input         string
	out           string
	temporal      bool
	workflowID    string
	temporalRunID string
	address       string
	namespace     string
	timeout       time.Duration
	traceDir      string
}

func parseTraceExportArgs(args []string) (traceExportOptions, error) {
	opts := traceExportOptions{
		address:   getenvDefault("TEMPORAL_ADDRESS", "localhost:7233"),
		namespace: getenvDefault("TEMPORAL_NAMESPACE", "default"),
		timeout:   10 * time.Second,
	}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--out", "-o":
			if i+1 >= len(args) {
				return traceExportOptions{}, fmt.Errorf("%s requires a value", args[i])
			}
			opts.out = args[i+1]
			i++
		case "--temporal":
			if i+1 >= len(args) {
				return traceExportOptions{}, fmt.Errorf("--temporal requires a workflow id")
			}
			opts.temporal = true
			opts.workflowID = args[i+1]
			i++
		case "--temporal-run-id":
			if i+1 >= len(args) {
				return traceExportOptions{}, fmt.Errorf("--temporal-run-id requires a value")
			}
			opts.temporalRunID = args[i+1]
			i++
		case "--address":
			if i+1 >= len(args) {
				return traceExportOptions{}, fmt.Errorf("--address requires a value")
			}
			opts.address = args[i+1]
			i++
		case "--namespace":
			if i+1 >= len(args) {
				return traceExportOptions{}, fmt.Errorf("--namespace requires a value")
			}
			opts.namespace = args[i+1]
			i++
		case "--timeout":
			if i+1 >= len(args) {
				return traceExportOptions{}, fmt.Errorf("--timeout requires a value")
			}
			d, err := time.ParseDuration(args[i+1])
			if err != nil || d <= 0 {
				return traceExportOptions{}, fmt.Errorf("invalid --timeout value %q", args[i+1])
			}
			opts.timeout = d
			i++
		case "--trace-dir":
			if i+1 >= len(args) {
				return traceExportOptions{}, fmt.Errorf("--trace-dir requires a value")
			}
			opts.traceDir = args[i+1]
			i++
		case "--help", "-h":
			printTraceUsage()
			os.Exit(0)
		default:
			if strings.HasPrefix(args[i], "-") && args[i] != "-" {
				return traceExportOptions{}, fmt.Errorf("unknown export argument %q", args[i])
			}
			if opts.input != "" {
				return traceExportOptions{}, fmt.Errorf("export accepts exactly one input trace path")
			}
			opts.input = args[i]
		}
	}
	if opts.temporal {
		if opts.workflowID == "" {
			return traceExportOptions{}, fmt.Errorf("--temporal requires a workflow id")
		}
		if opts.input != "" {
			return traceExportOptions{}, fmt.Errorf("export accepts either an input trace path or --temporal, not both")
		}
		return opts, nil
	}
	if opts.input == "" {
		return traceExportOptions{}, fmt.Errorf("export requires an input trace path")
	}
	return opts, nil
}

func exportLocalTraceArtifact(opts traceExportOptions) (*traceutil.Artifact, error) {
	if opts.input == "-" {
		artifact, err := traceutil.ReadFile(opts.input)
		if err != nil {
			return nil, fmt.Errorf("read trace: %w", err)
		}
		return artifact, nil
	}
	if _, err := os.Stat(opts.input); err == nil {
		artifact, readErr := traceutil.ReadFile(opts.input)
		if readErr != nil {
			return nil, fmt.Errorf("read trace: %w", readErr)
		}
		return artifact, nil
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("stat trace input: %w", err)
	}
	artifact, path, err := findLocalTraceByRunID(opts.input, traceLookupDirs(opts.traceDir))
	if err != nil {
		return nil, err
	}
	if path != "" {
		fmt.Fprintf(os.Stderr, "gollem: exporting local trace %s for run %s\n", path, opts.input)
	}
	return artifact, nil
}

func traceLookupDirs(explicit string) []string {
	var dirs []string
	add := func(value string) {
		for _, dir := range filepath.SplitList(value) {
			dir = strings.TrimSpace(dir)
			if dir == "" {
				continue
			}
			for _, existing := range dirs {
				if existing == dir {
					return
				}
			}
			dirs = append(dirs, dir)
		}
	}
	add(explicit)
	add(os.Getenv("GOLLEM_TRACE_DIR"))
	add(os.Getenv("GOLLEM_TRACE_DIRS"))
	add("/tmp/gollem-traces")
	return dirs
}

func findLocalTraceByRunID(runID string, dirs []string) (*traceutil.Artifact, string, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, "", fmt.Errorf("local trace export requires a trace path or run id")
	}
	var bestArtifact *traceutil.Artifact
	var bestPath string
	var bestStarted time.Time
	for _, dir := range dirs {
		if strings.TrimSpace(dir) == "" {
			continue
		}
		err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry == nil || entry.IsDir() {
				return nil
			}
			if !strings.HasSuffix(entry.Name(), ".trace.json") {
				return nil
			}
			artifact, readErr := traceutil.ReadFile(path)
			if readErr == nil && traceMatchesRunID(artifact, runID) {
				started := artifact.Run.StartedAt
				if started.IsZero() && artifact.Trace != nil {
					started = artifact.Trace.StartTime
				}
				if bestArtifact == nil || started.After(bestStarted) {
					bestArtifact = artifact
					bestPath = path
					bestStarted = started
				}
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return nil, "", fmt.Errorf("scan trace dir %s: %w", dir, err)
		}
	}
	if bestArtifact == nil {
		return nil, "", fmt.Errorf("local trace run %q not found in trace dirs: %s", runID, strings.Join(dirs, ", "))
	}
	return bestArtifact, bestPath, nil
}

func traceMatchesRunID(artifact *traceutil.Artifact, runID string) bool {
	if artifact == nil {
		return false
	}
	if artifact.Run.ID == runID {
		return true
	}
	return artifact.Trace != nil && artifact.Trace.RunID == runID
}

func parseTraceInputOutput(args []string) (input string, out string, err error) {
	opts, err := parseTraceExportArgs(args)
	if err != nil {
		return "", "", err
	}
	return opts.input, opts.out, nil
}

func queryTemporalTraceArtifact(opts traceExportOptions) (*traceutil.Artifact, error) {
	ctx, cancel := context.WithTimeout(context.Background(), opts.timeout)
	defer cancel()

	c, err := client.Dial(client.Options{
		HostPort:  opts.address,
		Namespace: opts.namespace,
	})
	if err != nil {
		return nil, fmt.Errorf("dial Temporal at %s (%s): %w", opts.address, opts.namespace, err)
	}
	defer c.Close()

	value, err := c.QueryWorkflow(ctx, opts.workflowID, opts.temporalRunID, temporalext.WorkflowStatusQueryName())
	if err != nil {
		return nil, fmt.Errorf("query workflow status: %w", err)
	}
	var status temporalext.WorkflowStatus
	if err := value.Get(&status); err != nil {
		return nil, fmt.Errorf("decode workflow status: %w", err)
	}
	artifact, err := traceutil.FromTemporalWorkflowStatus(&status, map[string]any{
		"temporal_address":     opts.address,
		"temporal_namespace":   opts.namespace,
		"temporal_workflow_id": opts.workflowID,
		"temporal_run_id":      opts.temporalRunID,
	})
	if err != nil {
		return nil, err
	}
	return artifact, nil
}

func getenvDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func readTraceTextOrFile(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if data, err := os.ReadFile(value); err == nil {
		return string(data), nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	return value, nil
}

func parseTraceInspectArgs(args []string) (input string, limit int, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--events":
			if i+1 >= len(args) {
				return "", 0, fmt.Errorf("--events requires a value")
			}
			if _, scanErr := fmt.Sscanf(args[i+1], "%d", &limit); scanErr != nil || limit < 0 {
				return "", 0, fmt.Errorf("invalid --events value %q", args[i+1])
			}
			i++
		case "--help", "-h":
			printTraceUsage()
			os.Exit(0)
		default:
			if strings.HasPrefix(args[i], "-") && args[i] != "-" {
				return "", 0, fmt.Errorf("unknown inspect argument %q", args[i])
			}
			if input != "" {
				return "", 0, fmt.Errorf("inspect accepts exactly one trace path")
			}
			input = args[i]
		}
	}
	if input == "" {
		return "", 0, fmt.Errorf("inspect requires a trace path")
	}
	return input, limit, nil
}

func parseTraceReplayArgs(args []string) (input string, mode string, err error) {
	mode = "strict"
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--mode":
			if i+1 >= len(args) {
				return "", "", fmt.Errorf("--mode requires a value")
			}
			mode = strings.TrimSpace(args[i+1])
			i++
		case "--help", "-h":
			printTraceUsage()
			os.Exit(0)
		default:
			if strings.HasPrefix(args[i], "-") && args[i] != "-" {
				return "", "", fmt.Errorf("unknown replay argument %q", args[i])
			}
			if input != "" {
				return "", "", fmt.Errorf("replay accepts exactly one trace path")
			}
			input = args[i]
		}
	}
	if input == "" {
		return "", "", fmt.Errorf("replay requires a trace path")
	}
	return input, mode, nil
}

func parseSingleTracePath(args []string) (string, error) {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		printTraceUsage()
		os.Exit(0)
	}
	if len(args) != 1 {
		return "", fmt.Errorf("expected exactly one trace path")
	}
	return args[0], nil
}

func parseTraceViewArgs(args []string) ([]string, error) {
	var paths []string
	for _, arg := range args {
		switch {
		case arg == "--help" || arg == "-h":
			printTraceUsage()
			os.Exit(0)
		case strings.HasPrefix(arg, "-") && arg != "-":
			return nil, fmt.Errorf("unknown view argument %q", arg)
		default:
			paths = append(paths, arg)
		}
	}
	if len(paths) != 1 && len(paths) != 2 {
		return nil, fmt.Errorf("view requires one trace path or baseline and variant trace paths")
	}
	return paths, nil
}

func printTraceUsage() {
	fmt.Fprintf(os.Stderr, `gollem trace - Inspect, replay, and diff Gollem traces

Usage:
  gollem trace <command> [options]

Commands:
  export <run-id|trace.json> [--trace-dir dir] [--out run.trace.json]
      Export a local run by run id from the trace directory, or convert a legacy core.RunTrace JSON file/existing artifact to gollem.trace.v1.

  export --temporal <workflow-id> [--temporal-run-id id] [--address host:port] [--namespace ns] [--out run.trace.json]
      Query a Temporal workflow's gollem.status and export its trace/snapshot as gollem.trace.v1.

  inspect <trace.json> [--events n]
      Print a compact trace summary and event timeline.

  view <trace.json>
      Open the terminal trace viewer.

  view <baseline.trace.json> <variant.trace.json>
      Open the terminal trace comparison viewer.

  replay <trace.json> [--mode inspect|strict|simulated|fork|live-reexec]
      Replay recorded runtime-boundary events into reconstructed state without pretending model sampling is deterministic.

  fork <trace.json> [--from-step n|--from-event id|--from-checkpoint id|--from-kind kind] [--system-prompt text-or-file] [--planner-prompt text-or-file] [--append-user text] [--model name] [--topology name] [--middleware name] [--tool-policy name] [--evaluator name] [--memory-edit key=value] [--out fork.snapshot.json]
      Extract a snapshot anchor for a branch run.

  fork <trace.json> --continue --out fork.trace.json [--provider name] [--workdir path] [--timeout duration] [--snapshot-out path] [--run-arg arg]
      Branch from a snapshot and immediately continue a fresh run segment that writes fork.trace.json.

  diff <baseline.trace.json> <variant.trace.json> [--format text|json]
      Compare two trace artifacts and show the first event divergence plus usage deltas.

  regress <baseline.trace.json> <variant.trace.json>... [--require-status status] [--max-token-delta n] [--max-cost-delta n] [--format text|json]
      Run trace-backed regression checks across one or more variants.

  sleepy <baseline.trace.json> <candidate.trace.json>... [--out evidence.json]
      Export Sleepy-compatible trace evidence for mutation ranking, drift checks, replay lineage, and evaluator-gaming detection.

  validate <trace.json>
      Validate schema, event sequence, replay policies, lineage, snapshots, and replay boundaries.

  redact <trace.json> [--key name] [--pattern text] [--drop-trace] [--out redacted.trace.json]
      Redact sensitive keys or literal values while preserving trace structure.

  compact <trace.json> [--payload-limit n] [--keep-snapshots n] [--keep-trace] [--out compact.trace.json]
      Shrink a trace artifact for sharing or archival.

Examples:
  gollem run --trace-out run.trace.json "Fix the failing tests"
  gollem trace export run_123 --trace-dir /tmp/gollem-traces --out run.trace.json
  gollem trace export --temporal workflow-123 --out workflow.trace.json
  gollem trace inspect run.trace.json
  gollem trace replay run.trace.json
  gollem trace fork run.trace.json --from-step 1 --append-user "try a cheaper plan" --out fork.snapshot.json
  gollem trace fork run.trace.json --from-step 1 --append-user "try a cheaper plan" --continue --provider test --out fork.trace.json
  gollem trace redact run.trace.json --pattern "$API_KEY" --out redacted.trace.json
  gollem trace compact run.trace.json --out compact.trace.json
  gollem run --resume-snapshot fork.snapshot.json --trace-out fork.trace.json "continue"
  gollem trace diff baseline.trace.json variant.trace.json
  gollem trace regress baseline.trace.json fork.trace.json --require-status succeeded
  gollem trace sleepy baseline.trace.json fork.trace.json --out sleepy-evidence.json
`)
}
