package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/codetool"
	"github.com/fugue-labs/gollem/modelutil"
	"github.com/fugue-labs/gollem/pkg/ui"
)

const (
	defaultServePort       = 8080
	defaultServeRunTimeout = 30 * time.Minute
)

type serveFlags struct {
	port        int
	provider    string
	modelName   string
	location    string
	project     string
	workDir     string
	tools       bool
	openBrowser bool
}

type serveRunConfig struct {
	provider  string
	modelName string
	workDir   string
	tools     bool
	timeout   time.Duration
	model     core.Model
}

func runServe() {
	f, err := parseServeFlags(os.Args[2:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		printServeUsage()
		os.Exit(1)
	}

	if f.provider == "" {
		f.provider = detectProvider()
		if f.provider == "" {
			fmt.Fprintln(os.Stderr, "error: --provider is required (or set ANTHROPIC_API_KEY / OPENAI_API_KEY)")
			os.Exit(1)
		}
	}

	requestTimeout := deriveRequestTimeout(defaultServeRunTimeout)
	baseModel, err := createModel(f.provider, f.modelName, f.location, f.project, requestTimeout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating model: %v\n", err)
		os.Exit(1)
	}
	if f.modelName == "" {
		f.modelName = strings.TrimSpace(baseModel.ModelName())
	}

	model := modelutil.NewRetryModel(baseModel, buildRetryConfig(f.provider, f.modelName, defaultServeRunTimeout))
	defer closeServeModel(model)

	runCfg := serveRunConfig{
		provider:  f.provider,
		modelName: f.modelName,
		workDir:   f.workDir,
		tools:     f.tools,
		timeout:   defaultServeRunTimeout,
		model:     model,
	}

	server, err := ui.NewServer(ui.WithRunStarter(newServeRunStarter(runCfg)))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating ui server: %v\n", err)
		os.Exit(1)
	}

	handler := withRunStartDefaults(server, ui.RunStartRequest{
		Provider: f.provider,
		Model:    f.modelName,
	})

	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	addr := fmt.Sprintf(":%d", f.port)
	dashboardURL := fmt.Sprintf("http://localhost:%d/", f.port)
	listener, err := (&net.ListenConfig{}).Listen(signalCtx, "tcp", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error listening on %s: %v\n", addr, err)
		os.Exit(1)
	}
	defer listener.Close()

	httpServer := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-signalCtx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	fmt.Fprintf(os.Stderr, "gollem: serving dashboard at %s (provider=%s, model=%s, tools=%t)\n", dashboardURL, f.provider, f.modelName, f.tools)
	if f.openBrowser {
		go func(target string) {
			if err := openBrowser(context.Background(), target); err != nil {
				fmt.Fprintf(os.Stderr, "warning: open browser: %v\n", err)
			}
		}(dashboardURL)
	}

	if err := httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintf(os.Stderr, "error serving ui: %v\n", err)
		os.Exit(1)
	}
}

func parseServeFlags(args []string) (serveFlags, error) {
	workDir, _ := os.Getwd()
	f := serveFlags{
		port:    defaultServePort,
		workDir: workDir,
	}

	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--provider":
			value, err := requireServeFlagValue(args, i, "--provider")
			if err != nil {
				return serveFlags{}, err
			}
			f.provider = strings.TrimSpace(value)
			i++
		case arg == "--model":
			value, err := requireServeFlagValue(args, i, "--model")
			if err != nil {
				return serveFlags{}, err
			}
			f.modelName = strings.TrimSpace(value)
			i++
		case arg == "--location":
			value, err := requireServeFlagValue(args, i, "--location")
			if err != nil {
				return serveFlags{}, err
			}
			f.location = strings.TrimSpace(value)
			i++
		case arg == "--project":
			value, err := requireServeFlagValue(args, i, "--project")
			if err != nil {
				return serveFlags{}, err
			}
			f.project = strings.TrimSpace(value)
			i++
		case arg == "--workdir":
			value, err := requireServeFlagValue(args, i, "--workdir")
			if err != nil {
				return serveFlags{}, err
			}
			f.workDir = strings.TrimSpace(value)
			i++
		case arg == "--port":
			value, err := requireServeFlagValue(args, i, "--port")
			if err != nil {
				return serveFlags{}, err
			}
			port, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || port <= 0 || port > 65535 {
				return serveFlags{}, fmt.Errorf("invalid --port value %q", value)
			}
			f.port = port
			i++
		case arg == "--tools":
			value, consumed, err := parseOptionalBoolFlag(args, i)
			if err != nil {
				return serveFlags{}, fmt.Errorf("--tools: %w", err)
			}
			f.tools = value
			i += consumed
		case strings.HasPrefix(arg, "--tools="):
			value, err := parseBoolString(strings.TrimPrefix(arg, "--tools="))
			if err != nil {
				return serveFlags{}, fmt.Errorf("--tools: %w", err)
			}
			f.tools = value
		case arg == "--open":
			value, consumed, err := parseOptionalBoolFlag(args, i)
			if err != nil {
				return serveFlags{}, fmt.Errorf("--open: %w", err)
			}
			f.openBrowser = value
			i += consumed
		case strings.HasPrefix(arg, "--open="):
			value, err := parseBoolString(strings.TrimPrefix(arg, "--open="))
			if err != nil {
				return serveFlags{}, fmt.Errorf("--open: %w", err)
			}
			f.openBrowser = value
		case arg == "--help" || arg == "-h":
			printServeUsage()
			os.Exit(0)
		default:
			return serveFlags{}, fmt.Errorf("unknown serve argument %q", arg)
		}
	}

	return f, nil
}

func requireServeFlagValue(args []string, index int, flag string) (string, error) {
	next := index + 1
	if next >= len(args) {
		return "", fmt.Errorf("%s requires a value", flag)
	}
	return args[next], nil
}

func parseOptionalBoolFlag(args []string, index int) (bool, int, error) {
	next := index + 1
	if next >= len(args) || strings.HasPrefix(args[next], "-") {
		return true, 0, nil
	}
	value, err := parseBoolString(args[next])
	if err != nil {
		return false, 0, err
	}
	return value, 1, nil
}

func parseBoolString(raw string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean value %q", raw)
	}
}

func newServeRunStarter(cfg serveRunConfig) ui.RunStarter {
	return ui.RunStarterFunc(func(ctx context.Context, runtime *ui.RunRuntime, req ui.RunStartRequest) error {
		runCfg := cfg
		runCfg.model = modelutil.NewSessionModel(cfg.model)

		agent := newServeAgent(runCfg, runtime)
		defer closeServeRunModels(runCfg.model, agent.GetModel())

		stream, err := agent.RunStream(ctx, req.Prompt, buildServeRunOptions(runCfg, req)...)
		if err != nil {
			return err
		}
		_, err = runtime.ConsumeStream(stream)
		return err
	})
}

func newServeAgent(cfg serveRunConfig, runtime *ui.RunRuntime) *core.Agent[string] {
	agentOpts := []core.AgentOption[string]{
		core.WithEventBus[string](runtime.EventBus),
		core.WithToolApproval[string](runtime.ApprovalBridge.ToolApprovalFunc()),
		core.WithRunCondition[string](core.MaxRunDuration(cfg.timeout)),
	}

	if cfg.tools {
		agentOpts = append(agentOpts,
			codetool.AgentOptions(cfg.workDir,
				codetool.WithModel(cfg.model),
				codetool.WithTimeout(cfg.timeout),
			)...,
		)
	} else {
		agentOpts = append(agentOpts,
			core.WithSystemPrompt[string]("You are a helpful assistant."),
		)
	}

	return core.NewAgent[string](cfg.model, agentOpts...)
}

func buildServeRunOptions(cfg serveRunConfig, req ui.RunStartRequest) []core.RunOption {
	var runOpts []core.RunOption
	if imageParts := detectPromptImageParts(req.Prompt, cfg.workDir); len(imageParts) > 0 && cfg.provider == "openai" {
		parts := make([]core.ModelRequestPart, 0, len(imageParts))
		for _, p := range imageParts {
			parts = append(parts, p)
		}
		runOpts = append(runOpts, core.WithInitialRequestParts(parts...))
	}
	return runOpts
}

func withRunStartDefaults(next http.Handler, defaults ui.RunStartRequest) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/runs/start" {
			next.ServeHTTP(w, r)
			return
		}

		updated, err := applyRunStartDefaults(r, defaults)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		next.ServeHTTP(w, updated)
	})
}

func applyRunStartDefaults(r *http.Request, defaults ui.RunStartRequest) (*http.Request, error) {
	clone := r.Clone(r.Context())
	contentType := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if strings.Contains(contentType, "application/json") {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, fmt.Errorf("read request body: %w", err)
		}
		_ = r.Body.Close()

		var req ui.RunStartRequest
		if len(bytes.TrimSpace(body)) > 0 {
			if err := json.Unmarshal(body, &req); err != nil {
				return nil, fmt.Errorf("invalid request body: %w", err)
			}
		}
		req.Provider = strings.TrimSpace(defaults.Provider)
		req.Model = strings.TrimSpace(defaults.Model)

		encoded, err := json.Marshal(req)
		if err != nil {
			return nil, fmt.Errorf("encode request body: %w", err)
		}
		clone.Body = io.NopCloser(bytes.NewReader(encoded))
		clone.ContentLength = int64(len(encoded))
		clone.Header = clone.Header.Clone()
		clone.Header.Set("Content-Type", "application/json")
		return clone, nil
	}

	if err := r.ParseForm(); err != nil {
		return nil, fmt.Errorf("invalid form body: %w", err)
	}
	form := make(url.Values, len(r.Form))
	for key, values := range r.Form {
		form[key] = append([]string(nil), values...)
	}
	form.Set("provider", strings.TrimSpace(defaults.Provider))
	form.Set("model", strings.TrimSpace(defaults.Model))
	encoded := form.Encode()
	clone.Body = io.NopCloser(strings.NewReader(encoded))
	clone.ContentLength = int64(len(encoded))
	clone.Header = clone.Header.Clone()
	clone.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	clone.Form = form
	clone.PostForm = form
	return clone, nil
}

func closeServeModel(model core.Model) {
	if closer, ok := model.(interface{ Close() error }); ok {
		_ = closer.Close()
	}
}

func closeServeRunModels(runModel, agentModel core.Model) {
	closeServeModel(agentModel)
	if !sameServeModelInstance(runModel, agentModel) {
		closeServeModel(runModel)
	}
}

func sameServeModelInstance(a, b core.Model) bool {
	if a == nil || b == nil {
		return a == b
	}
	ta := reflect.TypeOf(a)
	tb := reflect.TypeOf(b)
	if ta != tb || !ta.Comparable() {
		return false
	}
	return a == b
}

func openBrowser(ctx context.Context, target string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		//nolint:gosec // target is an internally-constructed localhost dashboard URL.
		cmd = exec.CommandContext(ctx, "open", target)
	case "windows":
		//nolint:gosec // target is an internally-constructed localhost dashboard URL.
		cmd = exec.CommandContext(ctx, "cmd", "/c", "start", "", target)
	default:
		//nolint:gosec // target is an internally-constructed localhost dashboard URL.
		cmd = exec.CommandContext(ctx, "xdg-open", target)
	}
	return cmd.Start()
}
