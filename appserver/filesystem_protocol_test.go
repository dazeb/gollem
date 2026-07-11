package appserver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/fugue-labs/gollem/appserver/protocol"
	toolfs "github.com/fugue-labs/gollem/appserver/tools/fs"
)

func TestServerFilesystemPublicContractAndLegacyCompatibility(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	fsSvc, err := toolfs.NewService(root)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	server := readyServer(WithFilesystem(fsSvc))

	publicPath := filepath.Join(root, "public.bin")
	writeResp := server.HandleRequest(ctx, request("fs/writeFile", map[string]any{
		"path": publicPath, "dataBase64": base64.StdEncoding.EncodeToString([]byte{0, 1, 2, 255}),
		"content": "public dataBase64 must win",
	}))
	if writeResp.Error != nil {
		t.Fatalf("public fs/writeFile error: %v", writeResp.Error)
	}
	var writeResult protocol.FsWriteFileResponse
	decodeResult(t, writeResp, &writeResult)

	readResp := server.HandleRequest(ctx, request("fs/readFile", map[string]any{"path": publicPath}))
	if readResp.Error != nil {
		t.Fatalf("public fs/readFile error: %v", readResp.Error)
	}
	var read protocol.FsReadFileResponse
	decodeResult(t, readResp, &read)
	if read.DataBase64 != "AAEC/w==" {
		t.Fatalf("public fs/readFile dataBase64 = %q", read.DataBase64)
	}

	legacyWrite := server.HandleRequest(ctx, request("fs/writeFile", map[string]any{
		"path": "legacy.txt", "content": "legacy",
	}))
	if legacyWrite.Error != nil {
		t.Fatalf("legacy fs/writeFile error: %v", legacyWrite.Error)
	}
	legacyRead := server.HandleRequest(ctx, request("fs/readFile", map[string]any{"path": "legacy.txt"}))
	var legacy fileContentResult
	decodeResult(t, legacyRead, &legacy)
	if legacy.Content != "legacy" || legacy.Encoding != "utf-8" {
		t.Fatalf("legacy fs/readFile = %+v", legacy)
	}

	missingChild := filepath.Join(root, "missing", "child")
	createResp := server.HandleRequest(ctx, request("fs/createDirectory", map[string]any{
		"path": missingChild, "recursive": false,
	}))
	if createResp.Error == nil {
		t.Fatal("non-recursive fs/createDirectory succeeded with missing parent")
	}
	createResp = server.HandleRequest(ctx, request("fs/createDirectory", map[string]any{"path": missingChild}))
	if createResp.Error != nil {
		t.Fatalf("default recursive fs/createDirectory error: %v", createResp.Error)
	}

	metadataResp := server.HandleRequest(ctx, request("fs/getMetadata", map[string]any{"path": publicPath}))
	if metadataResp.Error != nil {
		t.Fatalf("public fs/getMetadata error: %v", metadataResp.Error)
	}
	var metadata protocol.FsGetMetadataResponse
	decodeResult(t, metadataResp, &metadata)
	if metadata.IsDirectory || !metadata.IsFile || metadata.IsSymlink || metadata.ModifiedAtMS == 0 {
		t.Fatalf("public fs/getMetadata = %+v", metadata)
	}
	if metadata.CreatedAtMS != 0 {
		t.Fatalf("public fs/getMetadata createdAtMs = %d, want unavailable sentinel 0", metadata.CreatedAtMS)
	}
	linkPath := filepath.Join(root, "public-link.bin")
	if err := os.Symlink(publicPath, linkPath); err != nil {
		t.Fatalf("Symlink public file: %v", err)
	}
	metadataResp = server.HandleRequest(ctx, request("fs/getMetadata", map[string]any{"path": linkPath}))
	decodeResult(t, metadataResp, &metadata)
	if !metadata.IsSymlink || !metadata.IsFile || metadata.IsDirectory {
		t.Fatalf("public symlink fs/getMetadata = %+v", metadata)
	}

	emptyDir := filepath.Join(root, "empty")
	if err := os.Mkdir(emptyDir, 0o755); err != nil {
		t.Fatalf("Mkdir empty: %v", err)
	}
	listResp := server.HandleRequest(ctx, request("fs/readDirectory", map[string]any{"path": emptyDir}))
	if listResp.Error != nil {
		t.Fatalf("public fs/readDirectory error: %v", listResp.Error)
	}
	var list protocol.FsReadDirectoryResponse
	decodeResult(t, listResp, &list)
	if list.Entries == nil || len(list.Entries) != 0 {
		t.Fatalf("public empty entries = %#v", list.Entries)
	}
	listResp = server.HandleRequest(ctx, request("fs/readDirectory", map[string]any{"path": root}))
	var rawList struct {
		Entries []map[string]any `json:"entries"`
	}
	decodeResult(t, listResp, &rawList)
	if len(rawList.Entries) == 0 || rawList.Entries[0]["Path"] == nil || rawList.Entries[0]["Name"] == nil || rawList.Entries[0]["IsDir"] == nil {
		t.Fatalf("legacy directory fields missing: %#v", rawList.Entries)
	}

	tree := filepath.Join(root, "tree")
	if err := os.MkdirAll(filepath.Join(tree, "nested"), 0o755); err != nil {
		t.Fatalf("MkdirAll tree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tree, "nested", "file.txt"), []byte("tree"), 0o644); err != nil {
		t.Fatalf("WriteFile tree: %v", err)
	}
	copyResp := server.HandleRequest(ctx, request("fs/copy", map[string]any{
		"sourcePath": tree, "destinationPath": filepath.Join(root, "flat-copy"),
	}))
	if copyResp.Error == nil {
		t.Fatal("public directory fs/copy succeeded without recursive")
	}
	copyResp = server.HandleRequest(ctx, request("fs/copy", map[string]any{
		"sourcePath": tree, "destinationPath": filepath.Join(root, "recursive-copy"), "recursive": true,
	}))
	if copyResp.Error != nil {
		t.Fatalf("public recursive fs/copy error: %v", copyResp.Error)
	}
	legacyCopy := server.HandleRequest(ctx, request("fs/copy", map[string]any{
		"source": "tree", "destination": "legacy-copy",
	}))
	if legacyCopy.Error != nil {
		t.Fatalf("legacy recursive fs/copy error: %v", legacyCopy.Error)
	}

	removeResp := server.HandleRequest(ctx, request("fs/remove", map[string]any{
		"path": tree, "recursive": false,
	}))
	if removeResp.Error == nil {
		t.Fatal("non-recursive fs/remove succeeded for populated directory")
	}
	missing := filepath.Join(root, "does-not-exist")
	removeResp = server.HandleRequest(ctx, request("fs/remove", map[string]any{
		"path": missing, "force": false,
	}))
	if removeResp.Error == nil {
		t.Fatal("non-force fs/remove succeeded for missing path")
	}
	removeResp = server.HandleRequest(ctx, request("fs/remove", map[string]any{
		"path": missing, "force": true,
	}))
	if removeResp.Error != nil {
		t.Fatalf("force fs/remove missing error: %v", removeResp.Error)
	}
}

func TestServerFilesystemRejectsMalformedPublicBase64(t *testing.T) {
	fsSvc, err := toolfs.NewService(t.TempDir())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	server := readyServer(WithFilesystem(fsSvc))
	resp := server.HandleRequest(context.Background(), request("fs/writeFile", map[string]any{
		"path": filepath.Join(fsSvc.Root(), "bad.bin"), "dataBase64": "not base64!",
	}))
	if resp.Error == nil || resp.Error.Code != protocol.CodeInvalidParams {
		t.Fatalf("malformed base64 response = %+v", resp)
	}
	if _, err := os.Stat(filepath.Join(fsSvc.Root(), "bad.bin")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("bad base64 file stat error = %v, want not exist", err)
	}
}

func TestServerFilesystemChangedPayloadsAreDistinct(t *testing.T) {
	root := t.TempDir()
	fsSvc, err := toolfs.NewService(root)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	defer fsSvc.Close()
	server := readyServer(WithFilesystem(fsSvc))

	writeResp := server.HandleRequest(context.Background(), request("fs/writeFile", map[string]any{
		"path": "legacy.txt", "content": "legacy",
	}))
	if writeResp.Error != nil {
		t.Fatalf("fs/writeFile error: %v", writeResp.Error)
	}
	notifications := server.DrainNotifications()
	var mutation protocol.FileChangedNotification
	if len(notifications) != 1 || json.Unmarshal(notifications[0].Params, &mutation) != nil || mutation.Operation != "writeFile" {
		t.Fatalf("mutation notifications = %+v, decoded = %+v", notifications, mutation)
	}

	watchPath := filepath.Join(root, "watched.txt")
	watchResp := server.HandleRequest(context.Background(), request("fs/watch", map[string]any{
		"watchId": "watch", "path": watchPath, "pollIntervalMillis": 20,
	}))
	if watchResp.Error != nil {
		t.Fatalf("fs/watch error: %v", watchResp.Error)
	}
	if err := os.WriteFile(watchPath, []byte("changed"), 0o644); err != nil {
		t.Fatalf("WriteFile watched: %v", err)
	}
	notification := waitForNotification(t, server, "fs/changed")
	var changed protocol.FsChangedNotification
	if err := json.Unmarshal(notification.Params, &changed); err != nil {
		t.Fatalf("decode watched fs/changed: %v", err)
	}
	if changed.WatchID != "watch" || len(changed.ChangedPaths) == 0 {
		t.Fatalf("watch notification = %+v", changed)
	}
}
