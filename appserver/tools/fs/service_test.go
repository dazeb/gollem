package fs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestServiceReadWriteDirectoryMetadata(t *testing.T) {
	ctx := context.Background()
	var events []AuditEvent
	svc := newTestService(t, WithAuditSink(func(ev AuditEvent) {
		events = append(events, ev)
	}))

	if err := svc.CreateDirectory(ctx, "notes"); err != nil {
		t.Fatalf("CreateDirectory: %v", err)
	}
	if err := svc.WriteFile(ctx, "notes/todo.txt", []byte("ship it\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	content, err := svc.ReadFile(ctx, "notes/todo.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(content.Content) != "ship it\n" || content.Path != "notes/todo.txt" || content.Mode.Perm() != 0o600 {
		t.Fatalf("content = %+v", content)
	}
	entries, err := svc.ReadDirectory(ctx, "notes")
	if err != nil {
		t.Fatalf("ReadDirectory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "todo.txt" {
		t.Fatalf("entries = %+v", entries)
	}
	meta, err := svc.Metadata(ctx, "notes/todo.txt")
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if meta.IsDir || meta.Size != int64(len("ship it\n")) {
		t.Fatalf("metadata = %+v", meta)
	}
	if len(events) < 5 {
		t.Fatalf("expected audit events, got %+v", events)
	}
	for _, ev := range events {
		if ev.At.IsZero() {
			t.Fatalf("audit event missing timestamp: %+v", ev)
		}
	}
}

func TestServiceRejectsTraversalAndSymlinkEscape(t *testing.T) {
	ctx := context.Background()
	outside := t.TempDir()
	svc := newTestService(t)

	if _, err := svc.ReadFile(ctx, "../outside.txt"); !errors.Is(err, ErrPathOutsideRoot) {
		t.Fatalf("traversal error = %v, want ErrPathOutsideRoot", err)
	}
	if err := os.Symlink(outside, filepath.Join(svc.Root(), "escape")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if _, err := svc.Metadata(ctx, "escape"); !errors.Is(err, ErrPathOutsideRoot) {
		t.Fatalf("symlink metadata error = %v, want ErrPathOutsideRoot", err)
	}
	if err := svc.WriteFile(ctx, "escape/file.txt", []byte("nope"), 0); !errors.Is(err, ErrPathOutsideRoot) {
		t.Fatalf("symlink write error = %v, want ErrPathOutsideRoot", err)
	}
}

func TestServiceApprovalAndAuditForMutations(t *testing.T) {
	ctx := context.Background()
	var events []AuditEvent
	denyRemove := func(_ context.Context, op Operation) error {
		if op.Kind == OperationRemove {
			return errors.New("remove disabled")
		}
		return nil
	}
	svc := newTestService(t, WithApproval(denyRemove), WithAuditSink(func(ev AuditEvent) {
		events = append(events, ev)
	}))
	if err := svc.WriteFile(ctx, "a.txt", []byte("a"), 0); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := svc.Remove(ctx, "a.txt"); !errors.Is(err, ErrApprovalDenied) {
		t.Fatalf("Remove error = %v, want ErrApprovalDenied", err)
	}
	if _, err := os.Stat(filepath.Join(svc.Root(), "a.txt")); err != nil {
		t.Fatalf("file should remain after denied remove: %v", err)
	}
	last := events[len(events)-1]
	if last.Operation.Kind != OperationRemove || last.Allowed || last.Err == "" {
		t.Fatalf("denied remove audit event = %+v", last)
	}
}

func TestServiceCopyFileAndDirectory(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	if err := svc.WriteFile(ctx, "src/file.txt", []byte("file"), 0o644); err != nil {
		t.Fatalf("WriteFile file: %v", err)
	}
	if err := svc.Copy(ctx, "src/file.txt", "dst/copied.txt"); err != nil {
		t.Fatalf("Copy file: %v", err)
	}
	copied, err := svc.ReadFile(ctx, "dst/copied.txt")
	if err != nil {
		t.Fatalf("Read copied file: %v", err)
	}
	if string(copied.Content) != "file" {
		t.Fatalf("copied content = %q", copied.Content)
	}

	if err := svc.WriteFile(ctx, "tree/a.txt", []byte("a"), 0); err != nil {
		t.Fatalf("WriteFile tree/a: %v", err)
	}
	if err := svc.WriteFile(ctx, "tree/nested/b.txt", []byte("b"), 0); err != nil {
		t.Fatalf("WriteFile tree/b: %v", err)
	}
	if err := svc.Copy(ctx, "tree", "tree-copy"); err != nil {
		t.Fatalf("Copy dir: %v", err)
	}
	nested, err := svc.ReadFile(ctx, "tree-copy/nested/b.txt")
	if err != nil {
		t.Fatalf("Read copied nested file: %v", err)
	}
	if string(nested.Content) != "b" {
		t.Fatalf("nested content = %q", nested.Content)
	}
}

func TestServiceRejectsUnsafeCopyDestinations(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	if err := svc.WriteFile(ctx, "same.txt", []byte("same"), 0); err != nil {
		t.Fatalf("WriteFile same: %v", err)
	}
	if err := svc.Copy(ctx, "same.txt", "same.txt"); !errors.Is(err, ErrInvalidCopyDestination) {
		t.Fatalf("Copy same file error = %v, want ErrInvalidCopyDestination", err)
	}
	if err := svc.WriteFile(ctx, "tree/a.txt", []byte("a"), 0); err != nil {
		t.Fatalf("WriteFile tree: %v", err)
	}
	if err := svc.Copy(ctx, "tree", "tree/nested/copy"); !errors.Is(err, ErrInvalidCopyDestination) {
		t.Fatalf("Copy dir into itself error = %v, want ErrInvalidCopyDestination", err)
	}
}

func TestServiceRemove(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	if err := svc.WriteFile(ctx, "gone/file.txt", []byte("bye"), 0); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := svc.Remove(ctx, "gone"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(filepath.Join(svc.Root(), "gone")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed path stat error = %v, want not exist", err)
	}
}

func TestServiceRefusesRootRemove(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	if err := svc.WriteFile(ctx, "keep.txt", []byte("keep"), 0); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := svc.Remove(ctx, "."); !errors.Is(err, ErrRefusingRoot) {
		t.Fatalf("Remove root error = %v, want ErrRefusingRoot", err)
	}
	if _, err := os.Stat(filepath.Join(svc.Root(), "keep.txt")); err != nil {
		t.Fatalf("file should remain after root remove refusal: %v", err)
	}
}

func TestServiceHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	svc := newTestService(t)
	if err := svc.WriteFile(ctx, "blocked.txt", []byte("blocked"), 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("WriteFile canceled error = %v, want context.Canceled", err)
	}
	if _, err := os.Stat(filepath.Join(svc.Root(), "blocked.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("blocked file stat error = %v, want not exist", err)
	}
}

func TestServiceWatchReportsMissingFileCreationAndUnwatchStops(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	defer svc.Close()
	events := make(chan WatchEvent, 4)
	watchPath := filepath.Join(svc.Root(), "watched.txt")
	result, err := svc.Watch(ctx, WatchRequest{
		WatchID:      "watch-file",
		Path:         watchPath,
		PollInterval: 20 * time.Millisecond,
	}, func(ev WatchEvent) {
		events <- ev
	})
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if result.Path != watchPath {
		t.Fatalf("watch result path = %q, want %q", result.Path, watchPath)
	}
	if err := os.WriteFile(watchPath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write watched file: %v", err)
	}
	event := waitForWatchEvent(t, events)
	if event.WatchID != "watch-file" || !containsPath(event.ChangedPaths, watchPath) {
		t.Fatalf("watch event = %#v", event)
	}
	if err := svc.Unwatch(ctx, "watch-file"); err != nil {
		t.Fatalf("Unwatch: %v", err)
	}
	drainWatchEvents(events)
	if err := os.WriteFile(watchPath, []byte("again"), 0o644); err != nil {
		t.Fatalf("write unwatched file: %v", err)
	}
	assertNoWatchEvent(t, events)
}

func TestServiceWatchDirectoryReportsChangedChildPath(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	defer svc.Close()
	events := make(chan WatchEvent, 4)
	watchDir := filepath.Join(svc.Root(), "dir")
	if err := os.MkdirAll(watchDir, 0o755); err != nil {
		t.Fatalf("mkdir watch dir: %v", err)
	}
	if _, err := svc.Watch(ctx, WatchRequest{
		WatchID:      "watch-dir",
		Path:         watchDir,
		PollInterval: 20 * time.Millisecond,
	}, func(ev WatchEvent) {
		events <- ev
	}); err != nil {
		t.Fatalf("Watch: %v", err)
	}
	child := filepath.Join(watchDir, "child.txt")
	if err := os.WriteFile(child, []byte("child"), 0o644); err != nil {
		t.Fatalf("write child: %v", err)
	}
	event := waitForWatchEvent(t, events)
	if event.WatchID != "watch-dir" || !containsPath(event.ChangedPaths, child) {
		t.Fatalf("watch event = %#v", event)
	}
}

func TestServiceWatchRejectsUnsafeRequests(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	defer svc.Close()
	if _, err := svc.Watch(ctx, WatchRequest{WatchID: "relative", Path: "relative.txt"}, nil); !errors.Is(err, ErrWatchPathNotAbsolute) {
		t.Fatalf("relative watch error = %v, want ErrWatchPathNotAbsolute", err)
	}
	if _, err := svc.Watch(ctx, WatchRequest{WatchID: "outside", Path: filepath.Join(t.TempDir(), "outside.txt")}, nil); !errors.Is(err, ErrPathOutsideRoot) {
		t.Fatalf("outside watch error = %v, want ErrPathOutsideRoot", err)
	}
	watchPath := filepath.Join(svc.Root(), "dup.txt")
	if _, err := svc.Watch(ctx, WatchRequest{WatchID: "dup", Path: watchPath}, nil); err != nil {
		t.Fatalf("first Watch: %v", err)
	}
	if _, err := svc.Watch(ctx, WatchRequest{WatchID: "dup", Path: watchPath}, nil); !errors.Is(err, ErrWatchAlreadyExists) {
		t.Fatalf("duplicate watch error = %v, want ErrWatchAlreadyExists", err)
	}
	if err := svc.Unwatch(ctx, "missing"); !errors.Is(err, ErrWatchNotFound) {
		t.Fatalf("missing unwatch error = %v, want ErrWatchNotFound", err)
	}
}

func newTestService(t *testing.T, opts ...Option) *Service {
	t.Helper()
	svc, err := NewService(t.TempDir(), opts...)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func waitForWatchEvent(t *testing.T, events <-chan WatchEvent) WatchEvent {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case ev := <-events:
			if len(ev.ChangedPaths) > 0 {
				return ev
			}
		case <-timeout:
			t.Fatal("timed out waiting for watch event")
		}
	}
}

func assertNoWatchEvent(t *testing.T, events <-chan WatchEvent) {
	t.Helper()
	select {
	case ev := <-events:
		t.Fatalf("unexpected watch event: %#v", ev)
	case <-time.After(150 * time.Millisecond):
	}
}

func drainWatchEvents(events <-chan WatchEvent) {
	for {
		select {
		case <-events:
		default:
			return
		}
	}
}

func containsPath(paths []string, want string) bool {
	for _, path := range paths {
		if path == want {
			return true
		}
	}
	return false
}
