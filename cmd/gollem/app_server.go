package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	appserver "github.com/fugue-labs/gollem/appserver"
	"github.com/fugue-labs/gollem/appserver/protocol"
	"github.com/fugue-labs/gollem/appserver/store"
	toolfs "github.com/fugue-labs/gollem/appserver/tools/fs"
	toolgit "github.com/fugue-labs/gollem/appserver/tools/git"
	toolprocess "github.com/fugue-labs/gollem/appserver/tools/process"
)

const defaultAppServerStoreRel = ".gollem/appserver.db"

type appServerFlags struct {
	workDir         string
	storePath       string
	gitRoot         string
	worktreeRoot    string
	stdio           bool
	allowMutations  bool
	gitRootExplicit bool
}

func runAppServer() {
	flags, err := parseAppServerFlags(os.Args[2:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		printAppServerUsage()
		os.Exit(1)
	}
	if !flags.stdio {
		fmt.Fprintln(os.Stderr, "error: only --stdio transport is currently supported")
		os.Exit(1)
	}

	server, cleanup, err := newCLIAppServer(flags)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating app-server: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := appserver.ServeJSONLines(ctx, server, os.Stdin, os.Stdout); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(os.Stderr, "error serving app-server: %v\n", err)
		os.Exit(1)
	}
}

func parseAppServerFlags(args []string) (appServerFlags, error) {
	workDir, _ := os.Getwd()
	flags := appServerFlags{workDir: workDir, stdio: true}

	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--stdio":
			value, consumed, err := parseOptionalBoolFlag(args, i)
			if err != nil {
				return appServerFlags{}, fmt.Errorf("--stdio: %w", err)
			}
			flags.stdio = value
			i += consumed
		case strings.HasPrefix(arg, "--stdio="):
			value, err := parseBoolString(strings.TrimPrefix(arg, "--stdio="))
			if err != nil {
				return appServerFlags{}, fmt.Errorf("--stdio: %w", err)
			}
			flags.stdio = value
		case arg == "--allow-mutations":
			value, consumed, err := parseOptionalBoolFlag(args, i)
			if err != nil {
				return appServerFlags{}, fmt.Errorf("--allow-mutations: %w", err)
			}
			flags.allowMutations = value
			i += consumed
		case strings.HasPrefix(arg, "--allow-mutations="):
			value, err := parseBoolString(strings.TrimPrefix(arg, "--allow-mutations="))
			if err != nil {
				return appServerFlags{}, fmt.Errorf("--allow-mutations: %w", err)
			}
			flags.allowMutations = value
		case arg == "--workdir":
			value, err := requireServeFlagValue(args, i, "--workdir")
			if err != nil {
				return appServerFlags{}, err
			}
			flags.workDir = strings.TrimSpace(value)
			i++
		case arg == "--store":
			value, err := requireServeFlagValue(args, i, "--store")
			if err != nil {
				return appServerFlags{}, err
			}
			flags.storePath = strings.TrimSpace(value)
			i++
		case arg == "--git-root":
			value, err := requireServeFlagValue(args, i, "--git-root")
			if err != nil {
				return appServerFlags{}, err
			}
			flags.gitRoot = strings.TrimSpace(value)
			flags.gitRootExplicit = true
			i++
		case arg == "--worktree-root":
			value, err := requireServeFlagValue(args, i, "--worktree-root")
			if err != nil {
				return appServerFlags{}, err
			}
			flags.worktreeRoot = strings.TrimSpace(value)
			i++
		case arg == "--help" || arg == "-h":
			printAppServerUsage()
			os.Exit(0)
		default:
			return appServerFlags{}, fmt.Errorf("unknown app-server argument %q", arg)
		}
	}
	if strings.TrimSpace(flags.workDir) == "" {
		return appServerFlags{}, errors.New("--workdir must not be empty")
	}
	return flags, nil
}

func newCLIAppServer(flags appServerFlags) (*appserver.Server, func(), error) {
	workDir, err := filepath.Abs(flags.workDir)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve workdir: %w", err)
	}
	storePath, err := resolveAppServerStorePath(workDir, flags.storePath)
	if err != nil {
		return nil, nil, err
	}
	st, err := store.NewSQLiteStore(storePath)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		_ = st.Close()
	}

	fsOpts := []toolfs.Option{}
	processOpts := []toolprocess.Option{}
	gitOpts := []toolgit.Option{}
	events := appserver.NewEventQueue()
	approvals := appserver.NewApprovalService()
	if !flags.allowMutations {
		fsOpts = append(fsOpts, toolfs.WithApproval(approvals.FilesystemApproval))
		processOpts = append(processOpts, toolprocess.WithApproval(approvals.ProcessApproval))
		gitOpts = append(gitOpts, toolgit.WithApproval(approvals.GitApproval))
	}
	processOpts = append(processOpts,
		toolprocess.WithOutputSink(func(ev toolprocess.OutputEvent) {
			method, params := appserver.ProcessOutputNotification(ev)
			events.Publish(method, params)
		}),
		toolprocess.WithExitSink(func(ev toolprocess.ExitEvent) {
			method, params := appserver.ProcessExitedNotification(ev)
			events.Publish(method, params)
		}),
	)

	fsSvc, err := toolfs.NewService(workDir, fsOpts...)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	processSvc, err := toolprocess.NewService(workDir, processOpts...)
	if err != nil {
		cleanup()
		return nil, nil, err
	}

	var gitSvc *toolgit.Service
	gitRoot := firstNonEmptyAppServer(flags.gitRoot, workDir)
	if flags.worktreeRoot != "" {
		gitOpts = append(gitOpts, toolgit.WithWorktreeRoot(flags.worktreeRoot))
	}
	gitSvc, err = toolgit.NewService(gitRoot, gitOpts...)
	if err != nil && flags.gitRootExplicit {
		cleanup()
		return nil, nil, err
	}
	if err != nil {
		gitSvc = nil
	}

	version := gitCommit
	if version == "" {
		version = "dev"
	}
	opts := []appserver.Option{
		appserver.WithImplementationInfo(protocol.ImplementationInfo{Name: "gollem-appserver", Version: version}),
		appserver.WithDaemonService(appserver.NewDaemonService(
			appserver.WithDaemonName("gollem-appserver"),
			appserver.WithDaemonVersion(version),
			appserver.WithDaemonTransport("stdio"),
			appserver.WithDaemonWorkDir(workDir),
			appserver.WithDaemonStorePath(storePath),
		)),
		appserver.WithStore(st),
		appserver.WithFilesystem(fsSvc),
		appserver.WithProcess(processSvc),
		appserver.WithEventQueue(events),
		appserver.WithApprovalService(approvals),
	}
	if gitSvc != nil {
		opts = append(opts, appserver.WithGit(gitSvc))
	}
	return appserver.NewServer(opts...), cleanup, nil
}

func resolveAppServerStorePath(workDir, configured string) (string, error) {
	if configured == "" {
		configured = filepath.Join(workDir, defaultAppServerStoreRel)
	}
	if configured == ":memory:" {
		return configured, nil
	}
	path := configured
	if !filepath.IsAbs(path) {
		path = filepath.Join(workDir, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve store path: %w", err)
	}
	//nolint:gosec // Store path is local operator-supplied daemon configuration.
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", fmt.Errorf("create store directory: %w", err)
	}
	return abs, nil
}

func firstNonEmptyAppServer(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func printAppServerUsage() {
	fmt.Fprintf(os.Stderr, `gollem app-server - Start the Codex-style JSON-RPC app-server daemon

Usage:
  gollem app-server [options]

Options:
  --stdio[=bool]            Serve newline-delimited JSON-RPC over stdio (default: true)
  --workdir <path>          Workspace root for fs/process tools (default: current directory)
  --store <path>            SQLite store path (default: <workdir>/.gollem/appserver.db)
  --git-root <path>         Git repository root (default: workdir; unavailable if not a repo)
  --worktree-root <path>    Directory where git/worktree/create may create worktrees
  --allow-mutations[=bool]  Bypass app-server approvals for fs/process/git mutations (default: false)
  -h, --help                Show this help

Protocol:
  One JSON object per line on stdin. Responses are written as one JSON object per line on stdout.
`)
}
