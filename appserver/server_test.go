package appserver

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appcache "github.com/fugue-labs/gollem/appserver/cache"
	"github.com/fugue-labs/gollem/appserver/catalog"
	"github.com/fugue-labs/gollem/appserver/protocol"
	"github.com/fugue-labs/gollem/appserver/store"
	toolfs "github.com/fugue-labs/gollem/appserver/tools/fs"
	toolgit "github.com/fugue-labs/gollem/appserver/tools/git"
	toolprocess "github.com/fugue-labs/gollem/appserver/tools/process"
)

func TestServerHandshakeAndUnavailable(t *testing.T) {
	ctx := context.Background()
	server := NewServer(WithImplementationInfo(protocol.ImplementationInfo{Name: "test-server", Version: "v1"}))

	preInit := server.HandleRequest(ctx, request("thread/list", nil))
	if preInit.Error == nil || preInit.Error.Code != protocol.CodeInvalidRequest {
		t.Fatalf("pre-init error = %#v, want invalid request", preInit.Error)
	}

	initResp := server.HandleRequest(ctx, request("initialize", protocol.InitializeParams{
		ClientInfo: protocol.ImplementationInfo{Name: "test-client"},
		Capabilities: protocol.InitializeCapabilities{
			OptOutNotificationMethods: []string{"thread/status/changed"},
		},
	}))
	if initResp.Error != nil {
		t.Fatalf("initialize error: %v", initResp.Error)
	}
	var initResult protocol.InitializeResponse
	decodeResult(t, initResp, &initResult)
	if initResult.ProtocolVersion != protocol.ProtocolVersion || initResult.ServerInfo.Name != "test-server" {
		t.Fatalf("initialize result = %#v", initResult)
	}
	if server.NotificationEnabled("thread/status/changed") {
		t.Fatal("NotificationEnabled returned true for opted-out method")
	}

	beforeReady := server.HandleRequest(ctx, request("thread/list", nil))
	if beforeReady.Error == nil || beforeReady.Error.Code != protocol.CodeInvalidRequest {
		t.Fatalf("before-ready error = %#v, want invalid request", beforeReady.Error)
	}
	if err := server.HandleNotification(ctx, protocol.Notification{Method: "initialized"}); err != nil {
		t.Fatalf("initialized notification: %v", err)
	}

	unknown := server.HandleRequest(ctx, request("not/a/method", nil))
	if unknown.Error == nil || unknown.Error.Code != protocol.CodeMethodNotFound {
		t.Fatalf("unknown method error = %#v, want method not found", unknown.Error)
	}
	knownMissingDependency := server.HandleRequest(ctx, request("thread/list", nil))
	if knownMissingDependency.Error == nil || knownMissingDependency.Error.Code != protocol.CodeMethodUnavailable {
		t.Fatalf("known missing dependency error = %#v, want unavailable", knownMissingDependency.Error)
	}
	missingProcess := server.HandleRequest(ctx, request("command/exec", map[string]any{"command": "echo hi"}))
	if missingProcess.Error == nil || missingProcess.Error.Code != protocol.CodeMethodUnavailable {
		t.Fatalf("missing process service error = %#v, want unavailable", missingProcess.Error)
	}

	repeatedInit := server.HandleRequest(ctx, request("initialize", nil))
	if repeatedInit.Error == nil || repeatedInit.Error.Code != protocol.CodeInvalidRequest {
		t.Fatalf("repeated initialize error = %#v, want invalid request", repeatedInit.Error)
	}
}

func TestServerThreadStoreHandlers(t *testing.T) {
	ctx := context.Background()
	st, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "threads.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	thread, err := st.CreateThread(ctx, store.CreateThreadRequest{Title: "Root", Workspace: "/tmp/work"})
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	turn, err := st.CreateTurn(ctx, store.CreateTurnRequest{ThreadID: thread.ID, Input: json.RawMessage(`{"text":"hi"}`)})
	if err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	if _, err := st.AppendItem(ctx, store.AppendItemRequest{ThreadID: thread.ID, TurnID: turn.ID, Kind: "message", Payload: json.RawMessage(`{"text":"hi"}`)}); err != nil {
		t.Fatalf("AppendItem: %v", err)
	}

	server := readyServer(WithStore(st))
	listResp := server.HandleRequest(ctx, request("thread/list", nil))
	if listResp.Error != nil {
		t.Fatalf("thread/list error: %v", listResp.Error)
	}
	var list struct {
		Threads []*store.Thread `json:"threads"`
	}
	decodeResult(t, listResp, &list)
	if len(list.Threads) != 1 || list.Threads[0].ID != thread.ID {
		t.Fatalf("thread/list = %#v", list.Threads)
	}

	readResp := server.HandleRequest(ctx, request("thread/read", map[string]any{"threadId": thread.ID}))
	if readResp.Error != nil {
		t.Fatalf("thread/read error: %v", readResp.Error)
	}
	var read threadReadResult
	decodeResult(t, readResp, &read)
	if read.Thread.ID != thread.ID || len(read.Turns) != 1 || len(read.Items) != 1 {
		t.Fatalf("thread/read = %#v", read)
	}

	forkResp := server.HandleRequest(ctx, request("thread/fork", map[string]any{
		"threadId":     thread.ID,
		"title":        "Fork",
		"includeItems": true,
	}))
	if forkResp.Error != nil {
		t.Fatalf("thread/fork error: %v", forkResp.Error)
	}
	var forked struct {
		Thread *store.Thread `json:"thread"`
	}
	decodeResult(t, forkResp, &forked)
	if forked.Thread.ForkedFromThreadID != thread.ID {
		t.Fatalf("forked thread = %#v", forked.Thread)
	}

	archiveResp := server.HandleRequest(ctx, request("thread/archive", map[string]any{"threadId": thread.ID}))
	if archiveResp.Error != nil {
		t.Fatalf("thread/archive error: %v", archiveResp.Error)
	}
	var archived struct {
		Thread *store.Thread `json:"thread"`
	}
	decodeResult(t, archiveResp, &archived)
	if archived.Thread.Status != store.ThreadArchived {
		t.Fatalf("archived status = %s", archived.Thread.Status)
	}
}

func TestServerCatalogHandlers(t *testing.T) {
	ctx := context.Background()
	catalogSvc := catalog.NewDefault(catalog.WithEnvLookup(func(key string) (string, bool) {
		if key == "OPENAI_API_KEY" {
			return "set", true
		}
		return "", false
	}))
	fsSvc, err := toolfs.NewService(t.TempDir())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	server := readyServer(WithCatalog(catalogSvc), WithFilesystem(fsSvc))

	providersResp := server.HandleRequest(ctx, request("provider/list", nil))
	if providersResp.Error != nil {
		t.Fatalf("provider/list error: %v", providersResp.Error)
	}
	var providers catalog.ProviderListResponse
	decodeResult(t, providersResp, &providers)
	if len(providers.Data) == 0 || !providerConfigured(providers.Data, catalog.ProviderOpenAI) {
		t.Fatalf("provider/list = %#v", providers.Data)
	}

	modelsResp := server.HandleRequest(ctx, request("model/list", map[string]any{
		"providerId": catalog.ProviderOpenAI,
		"limit":      2,
	}))
	if modelsResp.Error != nil {
		t.Fatalf("model/list error: %v", modelsResp.Error)
	}
	var models catalog.ModelListResponse
	decodeResult(t, modelsResp, &models)
	if len(models.Data) != 2 || models.NextCursor == nil {
		t.Fatalf("model/list = %#v", models)
	}
	if models.Data[0].ProviderID != catalog.ProviderOpenAI || !models.Data[0].IsDefault {
		t.Fatalf("model/list first model = %#v", models.Data[0])
	}

	codexCapsResp := server.HandleRequest(ctx, request("modelProvider/capabilities/read", map[string]any{
		"providerId": catalog.ProviderOpenAI,
	}))
	if codexCapsResp.Error != nil {
		t.Fatalf("modelProvider/capabilities/read error: %v", codexCapsResp.Error)
	}
	var codexCaps catalog.ProviderCapabilities
	decodeResult(t, codexCapsResp, &codexCaps)
	if !codexCaps.NamespaceTools || !codexCaps.ToolCalls || !codexCaps.Configured {
		t.Fatalf("codex capabilities = %#v", codexCaps)
	}

	aliasResp := server.HandleRequest(ctx, request("provider/capabilities/read", map[string]any{
		"provider": catalog.ProviderAnthropic,
	}))
	if aliasResp.Error != nil {
		t.Fatalf("provider/capabilities/read error: %v", aliasResp.Error)
	}
	var aliasCaps catalog.ProviderCapabilities
	decodeResult(t, aliasResp, &aliasCaps)
	if !aliasCaps.AdaptiveThinking || !aliasCaps.ManualThinking {
		t.Fatalf("anthropic capabilities = %#v", aliasCaps)
	}

	toolsResp := server.HandleRequest(ctx, request("tool/list", map[string]any{"includeUnavailable": true}))
	if toolsResp.Error != nil {
		t.Fatalf("tool/list error: %v", toolsResp.Error)
	}
	var tools catalog.ToolListResponse
	decodeResult(t, toolsResp, &tools)
	if !toolAvailable(tools.Data, "fs") {
		t.Fatalf("tool/list did not report filesystem available: %#v", tools.Data)
	}
	if toolAvailable(tools.Data, "git") {
		t.Fatalf("tool/list reported git available without git service: %#v", tools.Data)
	}
	if !toolAvailable(tools.Data, "cache") {
		t.Fatalf("tool/list did not report cache available: %#v", tools.Data)
	}
}

func TestServerCacheHandlers(t *testing.T) {
	ctx := context.Background()
	server := readyServer()

	statsResp := server.HandleRequest(ctx, request("cache/stats", nil))
	if statsResp.Error != nil {
		t.Fatalf("cache/stats error: %v", statsResp.Error)
	}
	var initial appcache.StatsResponse
	decodeResult(t, statsResp, &initial)
	if initial.TotalRequests != 0 {
		t.Fatalf("initial cache stats = %#v", initial)
	}

	benchmarkResp := server.HandleRequest(ctx, request("cache/benchmark", map[string]any{
		"includeEvents": true,
	}))
	if benchmarkResp.Error != nil {
		t.Fatalf("cache/benchmark error: %v", benchmarkResp.Error)
	}
	var benchmark appcache.BenchmarkResponse
	decodeResult(t, benchmarkResp, &benchmark)
	if !benchmark.Passed {
		t.Fatalf("cache benchmark failed: %#v", benchmark)
	}
	if len(benchmark.Providers) != 2 {
		t.Fatalf("cache benchmark providers = %#v", benchmark.Providers)
	}
	for _, provider := range benchmark.Providers {
		if provider.HitRate < 0.90 {
			t.Fatalf("%s hit rate = %f, want >= .90", provider.Provider, provider.HitRate)
		}
	}
	if len(benchmark.Events) == 0 {
		t.Fatal("cache benchmark did not return typed events")
	}

	afterResp := server.HandleRequest(ctx, request("cache/stats", nil))
	if afterResp.Error != nil {
		t.Fatalf("cache/stats after benchmark error: %v", afterResp.Error)
	}
	var after appcache.StatsResponse
	decodeResult(t, afterResp, &after)
	if after.TotalRequests != benchmark.Totals.TotalRequests || after.HitRate < 0.90 {
		t.Fatalf("cache stats after benchmark = %#v, benchmark = %#v", after, benchmark.Totals)
	}
}

func TestServerFilesystemHandlers(t *testing.T) {
	ctx := context.Background()
	fsSvc, err := toolfs.NewService(t.TempDir())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	server := readyServer(WithFilesystem(fsSvc))

	writeResp := server.HandleRequest(ctx, request("fs/writeFile", map[string]any{
		"path":    "nested/hello.txt",
		"content": "hello",
	}))
	if writeResp.Error != nil {
		t.Fatalf("fs/writeFile error: %v", writeResp.Error)
	}

	readResp := server.HandleRequest(ctx, request("fs/readFile", map[string]any{"path": "nested/hello.txt"}))
	if readResp.Error != nil {
		t.Fatalf("fs/readFile error: %v", readResp.Error)
	}
	var read fileContentResult
	decodeResult(t, readResp, &read)
	if read.Content != "hello" || read.Encoding != "utf-8" || read.Path != "nested/hello.txt" {
		t.Fatalf("fs/readFile = %#v", read)
	}

	copyResp := server.HandleRequest(ctx, request("fs/copy", map[string]any{
		"source":      "nested/hello.txt",
		"destination": "copy.txt",
	}))
	if copyResp.Error != nil {
		t.Fatalf("fs/copy error: %v", copyResp.Error)
	}
	listResp := server.HandleRequest(ctx, request("fs/readDirectory", map[string]any{"path": "."}))
	if listResp.Error != nil {
		t.Fatalf("fs/readDirectory error: %v", listResp.Error)
	}
}

func TestServerProcessHandlers(t *testing.T) {
	ctx := context.Background()
	processSvc, err := toolprocess.NewService(t.TempDir())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	server := readyServer(WithProcess(processSvc))

	startResp := server.HandleRequest(ctx, request("process/spawn", map[string]any{
		"command": "cat",
	}))
	if startResp.Error != nil {
		t.Fatalf("process/spawn error: %v", startResp.Error)
	}
	var started struct {
		Process processSnapshotResult `json:"process"`
	}
	decodeResult(t, startResp, &started)
	if started.Process.ID == "" {
		t.Fatalf("process/spawn result = %#v", started.Process)
	}

	writeResp := server.HandleRequest(ctx, request("process/writeStdin", map[string]any{
		"id":    started.Process.ID,
		"data":  "hello\n",
		"close": true,
	}))
	if writeResp.Error != nil {
		t.Fatalf("process/writeStdin error: %v", writeResp.Error)
	}
	snapshot, err := processSvc.Wait(ctx, started.Process.ID)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if string(snapshot.Stdout) != "hello\n" {
		t.Fatalf("stdout = %q", snapshot.Stdout)
	}

	resizeResp := server.HandleRequest(ctx, request("process/resizePty", map[string]any{
		"id":   started.Process.ID,
		"cols": 80,
		"rows": 24,
	}))
	if resizeResp.Error == nil || resizeResp.Error.Code != protocol.CodeMethodUnavailable {
		t.Fatalf("resize error = %#v, want unavailable", resizeResp.Error)
	}
}

func TestServerGitHandlers(t *testing.T) {
	ctx := context.Background()
	repo := initRepo(t)
	gitSvc, err := toolgit.NewService(repo, toolgit.WithWorktreeRoot(filepath.Join(t.TempDir(), "worktrees")))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	server := readyServer(WithGit(gitSvc))

	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("changed\n"), 0o644); err != nil {
		t.Fatalf("write changed file: %v", err)
	}
	statusResp := server.HandleRequest(ctx, request("git/status", nil))
	if statusResp.Error != nil {
		t.Fatalf("git/status error: %v", statusResp.Error)
	}
	var status struct {
		Status *toolgit.Status `json:"status"`
	}
	decodeResult(t, statusResp, &status)
	if status.Status.Clean || len(status.Status.Entries) != 1 {
		t.Fatalf("git/status = %#v", status.Status)
	}

	diffResp := server.HandleRequest(ctx, request("git/diff", nil))
	if diffResp.Error != nil {
		t.Fatalf("git/diff error: %v", diffResp.Error)
	}
	var diff struct {
		Diff *toolgit.Diff `json:"diff"`
	}
	decodeResult(t, diffResp, &diff)
	if diff.Diff.Patch == "" {
		t.Fatal("git/diff returned empty patch")
	}

	commitResp := server.HandleRequest(ctx, request("git/commit", map[string]any{
		"message": "commit changed file",
		"all":     true,
	}))
	if commitResp.Error != nil {
		t.Fatalf("git/commit error: %v", commitResp.Error)
	}
	var commit struct {
		Commit *toolgit.CommitResult `json:"commit"`
	}
	decodeResult(t, commitResp, &commit)
	if commit.Commit.Hash == "" {
		t.Fatalf("git/commit = %#v", commit.Commit)
	}

	listResp := server.HandleRequest(ctx, request("git/worktree/list", nil))
	if listResp.Error != nil {
		t.Fatalf("git/worktree/list error: %v", listResp.Error)
	}
}

func readyServer(opts ...Option) *Server {
	server := NewServer(opts...)
	_ = server.HandleRequest(context.Background(), request("initialize", protocol.InitializeParams{
		ClientInfo: protocol.ImplementationInfo{Name: "test-client"},
	}))
	if err := server.HandleNotification(context.Background(), protocol.Notification{Method: "initialized"}); err != nil {
		panic(err)
	}
	return server
}

func request(method string, params any) protocol.Request {
	var raw json.RawMessage
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			panic(err)
		}
		raw = data
	}
	return protocol.Request{
		ID:     protocol.NewStringID("req-" + method),
		Method: method,
		Params: raw,
	}
}

func decodeResult(t *testing.T, resp protocol.Response, out any) {
	t.Helper()
	if resp.Error != nil {
		t.Fatalf("response error: %v", resp.Error)
	}
	if err := json.Unmarshal(resp.Result, out); err != nil {
		t.Fatalf("unmarshal result %s: %v", string(resp.Result), err)
	}
}

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "initial")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = cleanTestGitEnv(os.Environ())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func cleanTestGitEnv(env []string) []string {
	blocked := map[string]struct{}{
		"GIT_ALTERNATE_OBJECT_DIRECTORIES": {},
		"GIT_COMMON_DIR":                   {},
		"GIT_DIR":                          {},
		"GIT_INDEX_FILE":                   {},
		"GIT_NAMESPACE":                    {},
		"GIT_OBJECT_DIRECTORY":             {},
		"GIT_PREFIX":                       {},
		"GIT_QUARANTINE_PATH":              {},
		"GIT_WORK_TREE":                    {},
	}
	out := make([]string, 0, len(env))
	for _, kv := range env {
		key, _, _ := strings.Cut(kv, "=")
		if _, ok := blocked[key]; ok {
			continue
		}
		out = append(out, kv)
	}
	return out
}

func providerConfigured(providers []catalog.Provider, id string) bool {
	for _, provider := range providers {
		if provider.ID == id {
			return provider.Configured
		}
	}
	return false
}

func toolAvailable(tools []catalog.Tool, id string) bool {
	for _, tool := range tools {
		if tool.ID == id {
			return tool.Available
		}
	}
	return false
}
