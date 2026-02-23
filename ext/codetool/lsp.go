package codetool

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/fugue-labs/gollem/core"
)

// LSPParams are the parameters for the lsp tool.
type LSPParams struct {
	// Method is the LSP method to invoke.
	Method string `json:"method" jsonschema:"description=LSP method: definition (go to definition)\\, references (find all usages)\\, hover (type info and docs)\\, diagnostics (errors/warnings in file)\\, symbols (workspace symbol search),enum=definition,enum=references,enum=hover,enum=diagnostics,enum=symbols"`

	// File is the target file path (relative or absolute).
	File string `json:"file" jsonschema:"description=File path (relative to working directory or absolute)"`

	// Line is the 1-indexed line number in the file.
	Line int `json:"line,omitempty" jsonschema:"description=Line number (1-indexed). Required for definition\\, references\\, hover."`

	// Character is the 1-indexed character offset on the line.
	Character int `json:"character,omitempty" jsonschema:"description=Character position on the line (1-indexed). Required for definition\\, references\\, hover."`

	// Query is a search string for workspace symbol search.
	Query string `json:"query,omitempty" jsonschema:"description=Search query for the symbols method."`
}

// lspServer manages a running language server process.
type lspServer struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	lang    string
	workDir string

	writeMu   sync.Mutex           // protects writes to stdin
	callMu    sync.Mutex           // serializes call() invocations
	nextID    atomic.Int64
	fileMu    sync.Mutex           // protects openFiles
	openFiles map[string]fileState // URI → state
	dead      atomic.Bool          // set when the process has exited

	// Background reader routes responses here.
	responses chan jsonrpcResponse
	readDone  chan struct{} // closed when readLoop exits
}

// fileState tracks the sync state of an opened file.
type fileState struct {
	version int
	mtime   int64 // os.FileInfo.ModTime().UnixNano()
}

// lspServerConfig describes how to start a language server.
type lspServerConfig struct {
	command     string   // binary name
	args        []string // arguments
	installHint string   // how to install
}

// languageForFile returns the LSP language ID for a file extension.
func languageForFile(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".go":
		return "go"
	case ".py", ".pyi":
		return "python"
	case ".ts", ".tsx", ".mts", ".cts":
		return "typescript"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	case ".rs":
		return "rust"
	case ".c", ".h":
		return "c"
	case ".cpp", ".cc", ".cxx", ".hpp", ".hxx", ".hh":
		return "cpp"
	case ".hs", ".lhs":
		return "haskell"
	case ".java":
		return "java"
	case ".rb":
		return "ruby"
	case ".lua":
		return "lua"
	case ".zig":
		return "zig"
	case ".cs":
		return "csharp"
	case ".kt", ".kts":
		return "kotlin"
	case ".swift":
		return "swift"
	case ".ex", ".exs":
		return "elixir"
	case ".scala", ".sc":
		return "scala"
	case ".php":
		return "php"
	case ".dart":
		return "dart"
	case ".ml", ".mli":
		return "ocaml"
	default:
		return ""
	}
}

// serverConfigs maps language IDs to server configurations.
// Ordered by preference — first available command wins.
var serverConfigs = map[string][]lspServerConfig{
	"go": {
		{command: "gopls", args: []string{"serve"}, installHint: "go install golang.org/x/tools/gopls@latest"},
	},
	"python": {
		{command: "pyright-langserver", args: []string{"--stdio"}, installHint: "pip install pyright"},
		{command: "pylsp", args: nil, installHint: "pip install python-lsp-server"},
	},
	"typescript": {
		{command: "typescript-language-server", args: []string{"--stdio"}, installHint: "npm i -g typescript-language-server typescript"},
	},
	"javascript": {
		{command: "typescript-language-server", args: []string{"--stdio"}, installHint: "npm i -g typescript-language-server typescript"},
	},
	"rust": {
		{command: "rust-analyzer", args: nil, installHint: "rustup component add rust-analyzer"},
	},
	"c": {
		{command: "clangd", args: nil, installHint: "apt install clangd"},
	},
	"cpp": {
		{command: "clangd", args: nil, installHint: "apt install clangd"},
	},
	"haskell": {
		{command: "haskell-language-server-wrapper", args: []string{"--lsp"}, installHint: "ghcup install hls"},
	},
	"zig": {
		{command: "zls", args: nil, installHint: "install zls from https://github.com/zigtools/zls"},
	},
	"ruby": {
		{command: "solargraph", args: []string{"stdio"}, installHint: "gem install solargraph"},
	},
	"lua": {
		{command: "lua-language-server", args: nil, installHint: "install lua-language-server"},
	},
	"java": {
		{command: "jdtls", args: nil, installHint: "install jdtls (Eclipse JDT Language Server)"},
	},
	"csharp": {
		{command: "OmniSharp", args: []string{"--languageserver"}, installHint: "install OmniSharp"},
	},
	"kotlin": {
		{command: "kotlin-language-server", args: nil, installHint: "install kotlin-language-server from https://github.com/fwcd/kotlin-language-server"},
	},
	"swift": {
		{command: "sourcekit-lsp", args: nil, installHint: "install via Xcode or swift toolchain"},
	},
	"elixir": {
		{command: "elixir-ls", args: nil, installHint: "install elixir-ls from https://github.com/elixir-lsp/elixir-ls"},
		{command: "nextls", args: []string{"--stdio"}, installHint: "mix escript.install hex next_ls"},
	},
	"scala": {
		{command: "metals", args: nil, installHint: "install metals from https://scalameta.org/metals/"},
	},
	"php": {
		{command: "intelephense", args: []string{"--stdio"}, installHint: "npm i -g intelephense"},
		{command: "phpactor", args: []string{"language-server"}, installHint: "install phpactor from https://phpactor.readthedocs.io/"},
	},
	"dart": {
		{command: "dart", args: []string{"language-server", "--protocol=lsp"}, installHint: "install Dart SDK from https://dart.dev/get-dart"},
	},
	"ocaml": {
		{command: "ocamllsp", args: nil, installHint: "opam install ocaml-lsp-server"},
	},
}

// findServerConfig returns the first available server config for a language.
func findServerConfig(lang string) (*lspServerConfig, error) {
	configs, ok := serverConfigs[lang]
	if !ok {
		return nil, fmt.Errorf("no language server configured for %q", lang)
	}
	for i := range configs {
		if _, err := exec.LookPath(configs[i].command); err == nil {
			return &configs[i], nil
		}
	}
	// Return the first config's install hint.
	return nil, fmt.Errorf("no language server found for %s — install with: %s", lang, configs[0].installHint)
}

// fileURI converts a file path to a file:// URI.
func fileURI(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return "file://" + abs
}

// uriToPath converts a file:// URI back to a file path.
func uriToPath(uri string) string {
	if strings.HasPrefix(uri, "file://") {
		parsed, err := url.Parse(uri)
		if err == nil {
			return parsed.Path
		}
		return strings.TrimPrefix(uri, "file://")
	}
	return uri
}

// --- JSON-RPC 2.0 wire protocol ---

type jsonrpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// jsonrpcReply is used to respond to server-initiated requests.
type jsonrpcReply struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result"`
}

// writeMessage writes an LSP JSON-RPC message with Content-Length framing.
func writeMessage(w io.Writer, data []byte) error {
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	if _, err := io.WriteString(w, header); err != nil {
		return err
	}
	_, err := w.Write(data)
	return err
}

// readMessage reads one LSP JSON-RPC message from the stream.
func readMessage(r *bufio.Reader) ([]byte, error) {
	// Read headers until empty line.
	contentLength := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("reading header: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			n, err := strconv.Atoi(val)
			if err != nil {
				return nil, fmt.Errorf("invalid Content-Length %q: %w", val, err)
			}
			contentLength = n
		}
	}
	if contentLength < 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, fmt.Errorf("reading body: %w", err)
	}
	return body, nil
}

// --- LSP server management ---

// startServer launches an LSP server process and performs the initialize handshake.
func startServer(ctx context.Context, lang, workDir string) (*lspServer, error) {
	cfg, err := findServerConfig(lang)
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, cfg.command, cfg.args...)
	cmd.Dir = workDir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	// Discard stderr to avoid blocking.
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting %s: %w", cfg.command, err)
	}

	srv := &lspServer{
		cmd:       cmd,
		stdin:     stdin,
		lang:      lang,
		workDir:   workDir,
		openFiles: make(map[string]fileState),
		responses: make(chan jsonrpcResponse, 16),
		readDone:  make(chan struct{}),
	}

	// Start background reader BEFORE the initialize handshake so that
	// server-initiated requests (e.g., client/registerCapability) are
	// handled automatically during initialization.
	go srv.readLoop(bufio.NewReaderSize(stdout, 256*1024))

	// Initialize handshake.
	if err := srv.initialize(ctx, workDir); err != nil {
		srv.kill()
		return nil, fmt.Errorf("LSP initialize: %w", err)
	}

	// Background goroutine: detect server crashes by calling Wait().
	// This sets srv.dead so getServer can detect and restart.
	go func() {
		srv.cmd.Wait() //nolint:errcheck
		srv.dead.Store(true)
	}()

	// Clean up when context is done.
	go func() {
		<-ctx.Done()
		srv.shutdown()
	}()

	return srv, nil
}

// readLoop runs in a background goroutine, reading LSP messages from stdout.
// It routes responses to the responses channel and auto-responds to
// server-initiated requests (e.g., client/registerCapability,
// window/workDoneProgress/create) that would otherwise cause deadlocks.
func (s *lspServer) readLoop(reader *bufio.Reader) {
	defer close(s.readDone)
	for {
		body, err := readMessage(reader)
		if err != nil {
			return // pipe closed or server died
		}

		var raw map[string]json.RawMessage
		if err := json.Unmarshal(body, &raw); err != nil {
			continue
		}

		rawID, hasID := raw["id"]
		_, hasMethod := raw["method"]
		_, hasResult := raw["result"]
		_, hasError := raw["error"]

		switch {
		case hasID && hasMethod && !hasResult && !hasError:
			// Server-initiated request — must respond or server blocks.
			s.respondToServerRequest(rawID)

		case hasID && (hasResult || hasError || !hasMethod):
			// Response to one of our calls.
			var resp jsonrpcResponse
			if err := json.Unmarshal(body, &resp); err == nil {
				s.responses <- resp
			}

		default:
			// Notification — discard.
		}
	}
}

// respondToServerRequest sends a null-result response to a server-initiated
// request. This handles common requests like client/registerCapability and
// window/workDoneProgress/create that servers send during initialization.
func (s *lspServer) respondToServerRequest(rawID json.RawMessage) {
	reply := jsonrpcReply{
		JSONRPC: "2.0",
		ID:      rawID,
		Result:  nil,
	}
	data, err := json.Marshal(reply)
	if err != nil {
		return
	}
	s.writeMu.Lock()
	writeMessage(s.stdin, data) //nolint:errcheck
	s.writeMu.Unlock()
}

// initialize sends the LSP initialize request and initialized notification.
func (s *lspServer) initialize(ctx context.Context, workDir string) error {
	absDir, _ := filepath.Abs(workDir)

	initParams := map[string]any{
		"processId": os.Getpid(),
		"rootUri":   fileURI(absDir),
		"capabilities": map[string]any{
			"textDocument": map[string]any{
				"definition":    map[string]any{},
				"references":    map[string]any{},
				"hover":         map[string]any{"contentFormat": []string{"plaintext", "markdown"}},
				"publishDiagnostics": map[string]any{},
				"synchronization": map[string]any{
					"didOpen":  true,
					"didClose": true,
					"didSave":  true,
				},
			},
			"workspace": map[string]any{
				"symbol": map[string]any{},
			},
		},
	}

	_, err := s.call(ctx, "initialize", initParams)
	if err != nil {
		return err
	}

	// Send initialized notification.
	return s.notify("initialized", map[string]any{})
}

// call sends a JSON-RPC request and waits for the response.
// Responses are delivered via the background readLoop goroutine,
// which also handles server-initiated requests automatically.
func (s *lspServer) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	s.callMu.Lock()
	defer s.callMu.Unlock()

	id := s.nextID.Add(1)
	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	s.writeMu.Lock()
	err = writeMessage(s.stdin, data)
	s.writeMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("writing request: %w", err)
	}

	// Wait for response via channel with proper timeout enforcement.
	// The background readLoop handles notifications and server requests.
	timeout := 30 * time.Second
	if method == "initialize" {
		timeout = 60 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case resp := <-s.responses:
			if resp.ID == id {
				if resp.Error != nil {
					return nil, fmt.Errorf("LSP error %d: %s", resp.Error.Code, resp.Error.Message)
				}
				return resp.Result, nil
			}
			// Stale response for a different ID — skip.

		case <-timer.C:
			return nil, fmt.Errorf("timeout waiting for response to %s (id=%d)", method, id)

		case <-ctx.Done():
			return nil, ctx.Err()

		case <-s.readDone:
			return nil, fmt.Errorf("language server process exited")
		}
	}
}

// notify sends a JSON-RPC notification (no response expected).
func (s *lspServer) notify(method string, params any) error {
	req := jsonrpcRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshaling notification: %w", err)
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return writeMessage(s.stdin, data)
}

// ensureFileOpen reads the file from disk and sends didOpen or didChange.
// Returns true if the file was actually synced (new or modified), false if unchanged.
func (s *lspServer) ensureFileOpen(filePath, uri, langID string) (bool, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return false, fmt.Errorf("stat file: %w", err)
	}
	mtime := info.ModTime().UnixNano()

	s.fileMu.Lock()
	state, opened := s.openFiles[uri]
	if opened && state.mtime == mtime {
		s.fileMu.Unlock()
		return false, nil // unchanged since last sync
	}
	s.fileMu.Unlock()

	content, err := os.ReadFile(filePath)
	if err != nil {
		return false, fmt.Errorf("reading file: %w", err)
	}

	s.fileMu.Lock()
	defer s.fileMu.Unlock()

	if !opened {
		s.openFiles[uri] = fileState{version: 1, mtime: mtime}
		return true, s.notify("textDocument/didOpen", map[string]any{
			"textDocument": map[string]any{
				"uri":        uri,
				"languageId": langID,
				"version":    1,
				"text":       string(content),
			},
		})
	}

	// File already open — send change with full content.
	version := state.version + 1
	s.openFiles[uri] = fileState{version: version, mtime: mtime}
	return true, s.notify("textDocument/didChange", map[string]any{
		"textDocument": map[string]any{
			"uri":     uri,
			"version": version,
		},
		"contentChanges": []map[string]any{
			{"text": string(content)},
		},
	})
}

// shutdown gracefully shuts down the server.
func (s *lspServer) shutdown() {
	// Best-effort shutdown request. Use a short timeout since
	// the server may already be dead (readDone closed).
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	s.call(ctx, "shutdown", nil) //nolint:errcheck
	s.notify("exit", nil)       //nolint:errcheck
	s.stdin.Close()

	// Wait briefly, then kill.
	done := make(chan struct{})
	go func() {
		s.cmd.Wait() //nolint:errcheck
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		s.kill()
	}
}

// kill forcefully kills the server process group.
func (s *lspServer) kill() {
	if s.cmd.Process != nil {
		syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL) //nolint:errcheck
	}
}

// --- LSP result types ---

type lspLocation struct {
	URI   string   `json:"uri"`
	Range lspRange `json:"range"`
}

type lspRange struct {
	Start lspPosition `json:"start"`
	End   lspPosition `json:"end"`
}

type lspPosition struct {
	Line      int `json:"line"`      // 0-indexed
	Character int `json:"character"` // 0-indexed
}

type lspHoverResult struct {
	Contents any      `json:"contents"`
	Range    lspRange `json:"range"`
}

type lspDiagnostic struct {
	Range    lspRange `json:"range"`
	Severity int      `json:"severity"` // 1=Error, 2=Warning, 3=Info, 4=Hint
	Message  string   `json:"message"`
	Source   string   `json:"source,omitempty"`
}

type lspSymbolInfo struct {
	Name     string      `json:"name"`
	Kind     int         `json:"kind"`
	Location lspLocation `json:"location"`
}

// --- Result formatting ---

// formatLocation formats a location relative to workDir.
func formatLocation(loc lspLocation, workDir string) string {
	path := uriToPath(loc.URI)
	if rel, err := filepath.Rel(workDir, path); err == nil && !strings.HasPrefix(rel, "..") {
		path = rel
	}
	return fmt.Sprintf("%s:%d:%d", path, loc.Range.Start.Line+1, loc.Range.Start.Character+1)
}

// readLineFromFile reads a specific line (0-indexed) from a file.
func readLineFromFile(path string, lineNum int) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for i := 0; scanner.Scan(); i++ {
		if i == lineNum {
			return strings.TrimRight(scanner.Text(), "\r\n")
		}
	}
	return ""
}

// formatLocations formats a list of locations with source lines.
func formatLocations(locs []lspLocation, workDir string, maxResults int) string {
	if len(locs) == 0 {
		return "No results found."
	}
	var b strings.Builder
	shown := len(locs)
	if maxResults > 0 && shown > maxResults {
		shown = maxResults
	}
	for i := 0; i < shown; i++ {
		loc := locs[i]
		path := uriToPath(loc.URI)
		relPath := path
		if rel, err := filepath.Rel(workDir, path); err == nil && !strings.HasPrefix(rel, "..") {
			relPath = rel
		}
		line := readLineFromFile(path, loc.Range.Start.Line)
		if line != "" {
			fmt.Fprintf(&b, "%s:%d: %s\n", relPath, loc.Range.Start.Line+1, strings.TrimSpace(line))
		} else {
			fmt.Fprintf(&b, "%s:%d:%d\n", relPath, loc.Range.Start.Line+1, loc.Range.Start.Character+1)
		}
	}
	if len(locs) > shown {
		fmt.Fprintf(&b, "... and %d more results\n", len(locs)-shown)
	}
	return strings.TrimRight(b.String(), "\n")
}

// formatHover formats hover result content.
func formatHover(result json.RawMessage) string {
	if string(result) == "null" || len(result) == 0 {
		return "No hover information available."
	}
	var hover lspHoverResult
	if err := json.Unmarshal(result, &hover); err != nil {
		return "No hover information available."
	}
	if hover.Contents == nil {
		return "No hover information available."
	}

	// Contents can be a string, MarkupContent, or MarkedString.
	switch v := hover.Contents.(type) {
	case string:
		return v
	case map[string]any:
		if val, ok := v["value"]; ok {
			return fmt.Sprintf("%v", val)
		}
		data, _ := json.Marshal(v)
		return string(data)
	case []any:
		var parts []string
		for _, item := range v {
			switch it := item.(type) {
			case string:
				parts = append(parts, it)
			case map[string]any:
				if val, ok := it["value"]; ok {
					parts = append(parts, fmt.Sprintf("%v", val))
				}
			}
		}
		return strings.Join(parts, "\n\n")
	default:
		data, _ := json.Marshal(hover.Contents)
		return string(data)
	}
}

// formatDiagnostics formats a list of diagnostics.
func formatDiagnostics(diags []lspDiagnostic, filePath string) string {
	if len(diags) == 0 {
		return "No diagnostics (no errors or warnings)."
	}
	severityName := map[int]string{1: "error", 2: "warning", 3: "info", 4: "hint"}
	var b strings.Builder
	for _, d := range diags {
		sev := severityName[d.Severity]
		if sev == "" {
			sev = "diagnostic"
		}
		fmt.Fprintf(&b, "%s:%d:%d: %s: %s\n",
			filePath, d.Range.Start.Line+1, d.Range.Start.Character+1,
			sev, d.Message)
	}
	return strings.TrimRight(b.String(), "\n")
}

// symbolKindName maps LSP SymbolKind to human-readable name.
func symbolKindName(kind int) string {
	names := map[int]string{
		1: "file", 2: "module", 3: "namespace", 4: "package",
		5: "class", 6: "method", 7: "property", 8: "field",
		9: "constructor", 10: "enum", 11: "interface", 12: "function",
		13: "variable", 14: "constant", 15: "string", 16: "number",
		17: "boolean", 18: "array", 19: "object", 20: "key",
		21: "null", 22: "enum_member", 23: "struct", 24: "event",
		25: "operator", 26: "type_parameter",
	}
	if name, ok := names[kind]; ok {
		return name
	}
	return "symbol"
}

// formatSymbols formats workspace symbol results.
func formatSymbols(symbols []lspSymbolInfo, workDir string) string {
	if len(symbols) == 0 {
		return "No symbols found."
	}
	var b strings.Builder
	shown := len(symbols)
	if shown > 50 {
		shown = 50
	}
	for i := 0; i < shown; i++ {
		sym := symbols[i]
		loc := formatLocation(sym.Location, workDir)
		fmt.Fprintf(&b, "%s [%s] — %s\n", sym.Name, symbolKindName(sym.Kind), loc)
	}
	if len(symbols) > shown {
		fmt.Fprintf(&b, "... and %d more symbols\n", len(symbols)-shown)
	}
	return strings.TrimRight(b.String(), "\n")
}

// --- Main tool ---

// LSP creates a tool that provides Language Server Protocol capabilities.
// The tool lazily starts language servers and manages their lifecycle.
// Servers are shut down when the context is cancelled.
func LSP(opts ...Option) core.Tool {
	cfg := applyOpts(opts)

	var mu sync.Mutex
	servers := map[string]*lspServer{} // keyed by language ID

	getServer := func(ctx context.Context, lang string) (*lspServer, error) {
		mu.Lock()
		defer mu.Unlock()

		if srv, ok := servers[lang]; ok {
			// Check if process is still alive.
			if !srv.dead.Load() {
				return srv, nil
			}
			// Dead — remove and restart.
			delete(servers, lang)
			fmt.Fprintf(os.Stderr, "[gollem] lsp: %s server crashed, restarting\n", lang)
		}

		workDir := cfg.WorkDir
		if workDir == "" {
			workDir = "."
		}

		srv, err := startServer(ctx, lang, workDir)
		if err != nil {
			return nil, err
		}
		servers[lang] = srv
		return srv, nil
	}

	return core.FuncTool[LSPParams](
		"lsp",
		"Query a Language Server for semantic code intelligence. "+
			"Methods: definition (go to definition), references (find all usages), "+
			"hover (type info and docs), diagnostics (errors/warnings in file), "+
			"symbols (workspace symbol search). "+
			"Requires a language server to be installed (e.g., gopls for Go, pyright for Python). "+
			"Use file+line+character for definition/references/hover. Use file for diagnostics. Use query for symbols.",
		func(ctx context.Context, params LSPParams) (string, error) {
			if params.File == "" && params.Method != "symbols" {
				return "", &core.ModelRetryError{Message: "file parameter is required"}
			}
			if params.Method == "" {
				return "", &core.ModelRetryError{Message: "method parameter is required (definition, references, hover, diagnostics, symbols)"}
			}

			// Resolve file path.
			filePath := params.File
			if filePath != "" && !filepath.IsAbs(filePath) && cfg.WorkDir != "" {
				filePath = filepath.Join(cfg.WorkDir, filePath)
			}

			// Determine language.
			var lang string
			if filePath != "" {
				lang = languageForFile(filePath)
			}
			if lang == "" && params.Method != "symbols" {
				return "", &core.ModelRetryError{
					Message: fmt.Sprintf("unsupported file type %q — LSP supports: .go, .py, .ts, .js, .rs, .c, .cpp, .hs, .java, .rb, .lua, .zig, .cs, .kt, .swift, .ex, .scala, .php, .dart, .ml", filepath.Ext(params.File)),
				}
			}
			if lang == "" {
				lang = "go" // Default for symbols-only queries.
			}

			// Get or start server.
			srv, err := getServer(ctx, lang)
			if err != nil {
				return "", &core.ModelRetryError{Message: err.Error()}
			}

			workDir := cfg.WorkDir
			if workDir == "" {
				workDir = "."
			}
			absWorkDir, _ := filepath.Abs(workDir)

			// Ensure file is synced with server.
			if filePath != "" {
				uri := fileURI(filePath)
				changed, err := srv.ensureFileOpen(filePath, uri, lang)
				if err != nil {
					return "", fmt.Errorf("syncing file: %w", err)
				}
				// Brief pause to let the server index, but only when
				// the file was actually new or modified.
				if changed {
					time.Sleep(200 * time.Millisecond)
				}
			}

			switch params.Method {
			case "definition":
				if params.Line == 0 || params.Character == 0 {
					return "", &core.ModelRetryError{Message: "line and character are required for definition (both 1-indexed)"}
				}
				return lspDefinition(ctx, srv, filePath, params.Line, params.Character, absWorkDir)

			case "references":
				if params.Line == 0 || params.Character == 0 {
					return "", &core.ModelRetryError{Message: "line and character are required for references (both 1-indexed)"}
				}
				return lspReferences(ctx, srv, filePath, params.Line, params.Character, absWorkDir)

			case "hover":
				if params.Line == 0 || params.Character == 0 {
					return "", &core.ModelRetryError{Message: "line and character are required for hover (both 1-indexed)"}
				}
				return lspHover(ctx, srv, filePath, params.Line, params.Character)

			case "diagnostics":
				return lspDiagnostics(ctx, srv, filePath, absWorkDir)

			case "symbols":
				query := params.Query
				if query == "" {
					return "", &core.ModelRetryError{Message: "query parameter is required for symbols method"}
				}
				return lspSymbols(ctx, srv, query, absWorkDir)

			default:
				return "", &core.ModelRetryError{
					Message: fmt.Sprintf("unknown method %q — use: definition, references, hover, diagnostics, symbols", params.Method),
				}
			}
		},
	)
}

// --- LSP method implementations ---

func lspDefinition(ctx context.Context, srv *lspServer, filePath string, line, char int, workDir string) (string, error) {
	result, err := srv.call(ctx, "textDocument/definition", map[string]any{
		"textDocument": map[string]any{"uri": fileURI(filePath)},
		"position":     map[string]any{"line": line - 1, "character": char - 1},
	})
	if err != nil {
		return "", err
	}

	// Result can be a Location, Location[], or null.
	var locs []lspLocation
	if err := json.Unmarshal(result, &locs); err != nil {
		// Try single location.
		var loc lspLocation
		if err2 := json.Unmarshal(result, &loc); err2 != nil {
			return "No definition found.", nil
		}
		locs = []lspLocation{loc}
	}
	if len(locs) == 0 {
		return "No definition found.", nil
	}
	return formatLocations(locs, workDir, 10), nil
}

func lspReferences(ctx context.Context, srv *lspServer, filePath string, line, char int, workDir string) (string, error) {
	result, err := srv.call(ctx, "textDocument/references", map[string]any{
		"textDocument": map[string]any{"uri": fileURI(filePath)},
		"position":     map[string]any{"line": line - 1, "character": char - 1},
		"context":      map[string]any{"includeDeclaration": true},
	})
	if err != nil {
		return "", err
	}

	var locs []lspLocation
	if err := json.Unmarshal(result, &locs); err != nil || len(locs) == 0 {
		return "No references found.", nil
	}

	header := fmt.Sprintf("%d reference(s) found:\n", len(locs))
	return header + formatLocations(locs, workDir, 30), nil
}

func lspHover(ctx context.Context, srv *lspServer, filePath string, line, char int) (string, error) {
	result, err := srv.call(ctx, "textDocument/hover", map[string]any{
		"textDocument": map[string]any{"uri": fileURI(filePath)},
		"position":     map[string]any{"line": line - 1, "character": char - 1},
	})
	if err != nil {
		return "", err
	}
	if string(result) == "null" || len(result) == 0 {
		return "No hover information available.", nil
	}
	return formatHover(result), nil
}

func lspDiagnostics(ctx context.Context, srv *lspServer, filePath, workDir string) (string, error) {
	// Diagnostics are pushed asynchronously via textDocument/publishDiagnostics.
	// We use a pull-based approach: call textDocument/diagnostic (LSP 3.17+).
	// If that fails, tell the user to build/compile instead.
	result, err := srv.call(ctx, "textDocument/diagnostic", map[string]any{
		"textDocument": map[string]any{"uri": fileURI(filePath)},
	})
	if err != nil {
		// Pull diagnostics not supported — fallback message.
		return "This language server doesn't support pull diagnostics. " +
			"Use bash to run the compiler/linter for diagnostics (e.g., 'go vet', 'pyright', 'tsc --noEmit').", nil
	}

	// Result is a DocumentDiagnosticReport with items or relatedDocuments.
	var report struct {
		Kind  string          `json:"kind"`
		Items []lspDiagnostic `json:"items"`
	}
	if err := json.Unmarshal(result, &report); err != nil {
		return "Failed to parse diagnostics.", nil
	}

	relPath := filePath
	if rel, err := filepath.Rel(workDir, filePath); err == nil && !strings.HasPrefix(rel, "..") {
		relPath = rel
	}
	return formatDiagnostics(report.Items, relPath), nil
}

func lspSymbols(ctx context.Context, srv *lspServer, query, workDir string) (string, error) {
	result, err := srv.call(ctx, "workspace/symbol", map[string]any{
		"query": query,
	})
	if err != nil {
		return "", err
	}

	var symbols []lspSymbolInfo
	if err := json.Unmarshal(result, &symbols); err != nil || len(symbols) == 0 {
		return "No symbols found.", nil
	}
	return formatSymbols(symbols, workDir), nil
}
