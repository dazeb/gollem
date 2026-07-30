package fs

import (
	"context"
	"errors"
	iofs "io/fs"
	"os"
	"path/filepath"
	"strings"
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

func TestServiceApprovalDenialCoversEveryMutationKind(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t, WithApproval(func(context.Context, Operation) error {
		return errors.New("denied")
	}))
	if err := os.WriteFile(filepath.Join(svc.Root(), "source.txt"), []byte("source"), 0o644); err != nil {
		t.Fatalf("WriteFile source: %v", err)
	}
	tests := []struct {
		name string
		run  func() error
	}{
		{"write", func() error { return svc.WriteFile(ctx, "written.txt", []byte("written"), 0o644) }},
		{"create directory", func() error { return svc.CreateDirectory(ctx, "created") }},
		{"copy", func() error { return svc.Copy(ctx, "source.txt", "copied.txt") }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); !errors.Is(err, ErrApprovalDenied) {
				t.Fatalf("denied mutation error = %v, want ErrApprovalDenied", err)
			}
		})
	}
}

func TestServiceRunApprovedMutationScopesExactOperations(t *testing.T) {
	ctx := context.Background()
	var approvals []Operation
	svc := newTestService(t, WithApproval(func(_ context.Context, op Operation) error {
		approvals = append(approvals, op)
		return nil
	}))
	if err := os.WriteFile(filepath.Join(svc.Root(), "remove.txt"), []byte("remove"), 0o644); err != nil {
		t.Fatalf("WriteFile remove fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(svc.Root(), "source.txt"), []byte("copy"), 0o640); err != nil {
		t.Fatalf("WriteFile copy fixture: %v", err)
	}
	tests := []struct {
		name string
		op   Operation
		run  func(context.Context) error
	}{
		{
			name: "write",
			op:   Operation{Kind: OperationWriteFile, Path: "written.txt"},
			run: func(mutationCtx context.Context) error {
				return svc.WriteFile(mutationCtx, "written.txt", []byte("written"), 0o600)
			},
		},
		{
			name: "create directory",
			op:   Operation{Kind: OperationCreateDirectory, Path: "created"},
			run: func(mutationCtx context.Context) error {
				return svc.CreateDirectory(mutationCtx, "created")
			},
		},
		{
			name: "copy",
			op:   Operation{Kind: OperationCopy, Path: "source.txt", Destination: "copied.txt"},
			run: func(mutationCtx context.Context) error {
				return svc.Copy(mutationCtx, "source.txt", "copied.txt")
			},
		},
		{
			name: "remove",
			op:   Operation{Kind: OperationRemove, Path: "remove.txt", Destructive: true},
			run: func(mutationCtx context.Context) error {
				return svc.Remove(mutationCtx, "remove.txt")
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			approvedBefore := len(approvals)
			if err := svc.RunApprovedMutation(ctx, test.op, func(mutationCtx context.Context) error {
				if len(approvals) != approvedBefore+1 || approvals[len(approvals)-1] != test.op {
					t.Fatalf("approval did not precede mutation: %+v", approvals)
				}
				return test.run(mutationCtx)
			}); err != nil {
				t.Fatalf("RunApprovedMutation: %v", err)
			}
		})
	}
	if data, err := os.ReadFile(filepath.Join(svc.Root(), "written.txt")); err != nil || string(data) != "written" {
		t.Fatalf("written file = %q, error %v", data, err)
	}
	if info, err := os.Stat(filepath.Join(svc.Root(), "created")); err != nil || !info.IsDir() {
		t.Fatalf("created directory = %+v, error %v", info, err)
	}
	if data, err := os.ReadFile(filepath.Join(svc.Root(), "copied.txt")); err != nil || string(data) != "copy" {
		t.Fatalf("copied file = %q, error %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(svc.Root(), "remove.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed file stat error = %v", err)
	}
}

func TestServiceRunApprovedMutationFailsClosed(t *testing.T) {
	ctx := context.Background()
	var nilService *Service
	if err := nilService.RunApprovedMutation(ctx, Operation{}, func(context.Context) error { return nil }); err == nil {
		t.Fatal("nil service accepted approved mutation")
	}
	svc := newTestService(t)
	if err := svc.RunApprovedMutation(ctx, Operation{Kind: OperationWriteFile, Path: "notes.txt"}, nil); err == nil {
		t.Fatal("nil callback was accepted")
	}
	if err := svc.RunApprovedMutation(ctx, Operation{Kind: OperationReadFile, Path: "notes.txt"}, func(context.Context) error { return nil }); !errors.Is(err, ErrInvalidMutationScope) {
		t.Fatalf("read operation scope error = %v, want ErrInvalidMutationScope", err)
	}
	if err := svc.RunApprovedMutation(ctx, Operation{Kind: OperationCopy, Path: "source.txt"}, func(context.Context) error { return nil }); err == nil {
		t.Fatal("copy scope without destination was accepted")
	}
	if err := svc.RunApprovedMutation(ctx, Operation{Kind: OperationCopy, Path: "source.txt", Destination: "../outside.txt"}, func(context.Context) error { return nil }); !errors.Is(err, ErrPathOutsideRoot) {
		t.Fatalf("outside copy destination error = %v, want ErrPathOutsideRoot", err)
	}
	if err := svc.RunApprovedMutation(ctx, Operation{Kind: OperationWriteFile, Path: "../outside.txt"}, func(context.Context) error { return nil }); !errors.Is(err, ErrPathOutsideRoot) {
		t.Fatalf("outside scope error = %v, want ErrPathOutsideRoot", err)
	}

	denied := newTestService(t, WithApproval(func(context.Context, Operation) error {
		return errors.New("denied")
	}))
	if err := denied.RunApprovedMutation(ctx, Operation{Kind: OperationWriteFile, Path: "notes.txt"}, func(context.Context) error {
		return errors.New("callback must not run")
	}); !errors.Is(err, ErrApprovalDenied) {
		t.Fatalf("denied scope error = %v, want ErrApprovalDenied", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := svc.RunApprovedMutation(canceled, Operation{Kind: OperationWriteFile, Path: "notes.txt"}, func(context.Context) error {
		return nil
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled scope error = %v, want context.Canceled", err)
	}
	approvalCtx, approvalCancel := context.WithCancel(ctx)
	cancelDuringApproval := newTestService(t, WithApproval(func(context.Context, Operation) error {
		approvalCancel()
		return nil
	}))
	if err := cancelDuringApproval.RunApprovedMutation(approvalCtx, Operation{Kind: OperationWriteFile, Path: "notes.txt"}, func(context.Context) error {
		return nil
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("post-approval cancellation error = %v, want context.Canceled", err)
	}
	callbackErr := errors.New("callback failed")
	if err := svc.RunApprovedMutation(ctx, Operation{Kind: OperationWriteFile, Path: "notes.txt"}, func(context.Context) error {
		return callbackErr
	}); !errors.Is(err, callbackErr) {
		t.Fatalf("callback error = %v, want %v", err, callbackErr)
	}
}

func TestServiceApprovedMutationScopeIsOneShot(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	op := Operation{Kind: OperationWriteFile, Path: "notes.txt"}
	var retained context.Context
	if err := svc.RunApprovedMutation(ctx, op, func(mutationCtx context.Context) error {
		retained = mutationCtx
		if err := svc.Remove(mutationCtx, "notes.txt"); !errors.Is(err, ErrInvalidMutationScope) {
			t.Fatalf("mismatched operation error = %v, want ErrInvalidMutationScope", err)
		}
		if err := svc.CreateDirectory(mutationCtx, "created"); !errors.Is(err, ErrInvalidMutationScope) {
			t.Fatalf("mismatched directory error = %v, want ErrInvalidMutationScope", err)
		}
		if err := svc.Copy(mutationCtx, "source.txt", "copied.txt"); !errors.Is(err, ErrInvalidMutationScope) {
			t.Fatalf("mismatched copy error = %v, want ErrInvalidMutationScope", err)
		}
		if err := svc.WriteFile(mutationCtx, "notes.txt", []byte("first"), 0o644); err != nil {
			return err
		}
		if err := svc.WriteFile(mutationCtx, "notes.txt", []byte("second"), 0o644); !errors.Is(err, ErrInvalidMutationScope) {
			t.Fatalf("reused operation error = %v, want ErrInvalidMutationScope", err)
		}
		return nil
	}); err != nil {
		t.Fatalf("RunApprovedMutation: %v", err)
	}
	if err := svc.WriteFile(retained, "notes.txt", []byte("late"), 0o644); !errors.Is(err, ErrInvalidMutationScope) {
		t.Fatalf("retained scope error = %v, want ErrInvalidMutationScope", err)
	}
	if data, err := os.ReadFile(filepath.Join(svc.Root(), "notes.txt")); err != nil || string(data) != "first" {
		t.Fatalf("one-shot content = %q, error %v", data, err)
	}
}

func TestServiceRevertFileRestoresExactStateAndMode(t *testing.T) {
	ctx := context.Background()
	var events []AuditEvent
	svc := newTestService(t, WithAuditSink(func(event AuditEvent) {
		events = append(events, event)
	}))
	path := filepath.Join(svc.Root(), "notes.txt")
	if err := os.WriteFile(path, []byte("after\n"), 0o644); err != nil {
		t.Fatalf("WriteFile fixture: %v", err)
	}
	result, err := svc.RevertFile(ctx, RevertFileRequest{
		Path:          "notes.txt",
		TransactionID: "restore-exact-state",
		Before: ExactFileState{
			Exists:  true,
			SHA256:  exactSHA256([]byte("before\n")),
			Content: []byte("before\n"),
			Mode:    0o600,
		},
		After: ExactFileState{
			Exists: true,
			SHA256: exactSHA256([]byte("after\n")),
		},
	})
	if err != nil {
		t.Fatalf("RevertFile: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile reverted: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat reverted: %v", err)
	}
	if string(content) != "before\n" || info.Mode().Perm() != 0o600 || !result.Restored || result.Removed {
		t.Fatalf("reverted content/mode/result = %q/%o/%+v", content, info.Mode().Perm(), result)
	}
	if len(events) != 1 || events[0].Operation.Kind != OperationRevertFileChange || !events[0].Allowed {
		t.Fatalf("revert audit events = %+v", events)
	}
}

func TestServiceRevertFileReversesCreateAndDelete(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		svc := newTestService(t)
		path := filepath.Join(svc.Root(), "created.txt")
		content := []byte("created\n")
		if err := os.WriteFile(path, content, 0o640); err != nil {
			t.Fatalf("WriteFile fixture: %v", err)
		}
		result, err := svc.RevertFile(context.Background(), RevertFileRequest{
			Path:          "created.txt",
			TransactionID: "reverse-create",
			Before:        ExactFileState{},
			After: ExactFileState{
				Exists: true, SHA256: exactSHA256(content), Mode: 0o640, CheckMode: true,
			},
		})
		if err != nil {
			t.Fatalf("RevertFile create: %v", err)
		}
		if !result.Removed || result.Restored || result.SHA256 != "" {
			t.Fatalf("create revert result = %+v", result)
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("created file stat after revert = %v, want not-exist", err)
		}
	})

	t.Run("delete", func(t *testing.T) {
		svc := newTestService(t)
		content := []byte("deleted\n")
		result, err := svc.RevertFile(context.Background(), RevertFileRequest{
			Path:          "deleted.txt",
			TransactionID: "reverse-delete",
			Before: ExactFileState{
				Exists: true, SHA256: exactSHA256(content), Content: content, Mode: 0o600, CheckMode: true,
			},
			After: ExactFileState{},
		})
		if err != nil {
			t.Fatalf("RevertFile delete: %v", err)
		}
		if !result.Restored || result.Removed || result.SHA256 != exactSHA256(content) {
			t.Fatalf("delete revert result = %+v", result)
		}
		path := filepath.Join(svc.Root(), "deleted.txt")
		restored, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile restored: %v", err)
		}
		info, err := os.Stat(path)
		if err != nil || string(restored) != string(content) || info.Mode().Perm() != 0o600 {
			t.Fatalf("restored file = %q mode=%o error=%v", restored, info.Mode().Perm(), err)
		}
	})
}

func TestServiceRevertFileRejectsStaleAndSymlinkStates(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	path := filepath.Join(svc.Root(), "notes.txt")
	if err := os.WriteFile(path, []byte("user edit\n"), 0o644); err != nil {
		t.Fatalf("WriteFile stale fixture: %v", err)
	}
	req := RevertFileRequest{
		Path:          "notes.txt",
		TransactionID: "reject-stale",
		Before:        ExactFileState{Exists: true, SHA256: exactSHA256([]byte("before\n")), Content: []byte("before\n"), Mode: 0o644},
		After:         ExactFileState{Exists: true, SHA256: exactSHA256([]byte("after\n"))},
	}
	if _, err := svc.RevertFile(ctx, req); !errors.Is(err, ErrExactStateMismatch) {
		t.Fatalf("stale RevertFile error = %v, want ErrExactStateMismatch", err)
	}
	content, _ := os.ReadFile(path)
	if string(content) != "user edit\n" {
		t.Fatalf("stale file changed to %q", content)
	}

	if err := os.Remove(path); err != nil {
		t.Fatalf("Remove stale fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(svc.Root(), "target.txt"), []byte("after\n"), 0o644); err != nil {
		t.Fatalf("WriteFile target: %v", err)
	}
	if err := os.Symlink("target.txt", path); err != nil {
		t.Fatalf("Symlink fixture: %v", err)
	}
	if _, err := svc.RevertFile(ctx, req); !errors.Is(err, ErrExactRevertSymlink) {
		t.Fatalf("symlink RevertFile error = %v, want ErrExactRevertSymlink", err)
	}
	target, _ := os.ReadFile(filepath.Join(svc.Root(), "target.txt"))
	if string(target) != "after\n" {
		t.Fatalf("symlink target changed to %q", target)
	}

	if err := os.Mkdir(filepath.Join(svc.Root(), "real"), 0o755); err != nil {
		t.Fatalf("Mkdir real: %v", err)
	}
	if err := os.WriteFile(filepath.Join(svc.Root(), "real", "nested.txt"), []byte("after\n"), 0o644); err != nil {
		t.Fatalf("WriteFile nested target: %v", err)
	}
	if err := os.Symlink("real", filepath.Join(svc.Root(), "linked")); err != nil {
		t.Fatalf("Symlink parent: %v", err)
	}
	req.Path = "linked/nested.txt"
	if _, err := svc.RevertFile(ctx, req); !errors.Is(err, ErrExactRevertSymlink) {
		t.Fatalf("parent-symlink RevertFile error = %v, want ErrExactRevertSymlink", err)
	}
	nested, _ := os.ReadFile(filepath.Join(svc.Root(), "real", "nested.txt"))
	if string(nested) != "after\n" {
		t.Fatalf("parent-symlink target changed to %q", nested)
	}
}

func TestServiceRevertFileRechecksAfterApproval(t *testing.T) {
	ctx := context.Background()
	var root string
	svc := newTestService(t, WithApproval(func(_ context.Context, operation Operation) error {
		if operation.Kind == OperationRevertFileChange {
			return os.WriteFile(filepath.Join(root, "notes.txt"), []byte("concurrent\n"), 0o644)
		}
		return nil
	}))
	root = svc.Root()
	path := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(path, []byte("after\n"), 0o644); err != nil {
		t.Fatalf("WriteFile fixture: %v", err)
	}
	_, err := svc.RevertFile(ctx, RevertFileRequest{
		Path:          "notes.txt",
		TransactionID: "recheck-after-approval",
		Before:        ExactFileState{Exists: true, SHA256: exactSHA256([]byte("before\n")), Content: []byte("before\n"), Mode: 0o644},
		After:         ExactFileState{Exists: true, SHA256: exactSHA256([]byte("after\n"))},
	})
	if !errors.Is(err, ErrExactStateMismatch) {
		t.Fatalf("RevertFile concurrent error = %v, want ErrExactStateMismatch", err)
	}
	content, _ := os.ReadFile(path)
	if string(content) != "concurrent\n" {
		t.Fatalf("concurrent file changed to %q", content)
	}
}

func TestServiceRevertFileHonorsCancellationAfterApproval(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	svc := newTestService(t, WithApproval(func(context.Context, Operation) error {
		cancel()
		return nil
	}))
	path := filepath.Join(svc.Root(), "notes.txt")
	after := []byte("after\n")
	if err := os.WriteFile(path, after, 0o644); err != nil {
		t.Fatalf("WriteFile fixture: %v", err)
	}
	_, err := svc.RevertFile(ctx, RevertFileRequest{
		Path:          "notes.txt",
		TransactionID: "cancel-after-approval",
		Before: ExactFileState{
			Exists: true, SHA256: exactSHA256([]byte("before\n")), Content: []byte("before\n"), Mode: 0o644,
		},
		After: ExactFileState{Exists: true, SHA256: exactSHA256(after)},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("post-approval cancellation error = %v, want context.Canceled", err)
	}
	content, _ := os.ReadFile(path)
	if string(content) != string(after) {
		t.Fatalf("post-approval cancellation changed file to %q", content)
	}
}

func TestServiceRevertFileRejectsMalformedDeniedAndUnsupportedRequests(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	path := filepath.Join(svc.Root(), "notes.txt")
	after := []byte("after\n")
	before := []byte("before\n")
	if err := os.WriteFile(path, after, 0o644); err != nil {
		t.Fatalf("WriteFile fixture: %v", err)
	}
	valid := RevertFileRequest{
		Path:          "notes.txt",
		TransactionID: "valid-request",
		Before: ExactFileState{
			Exists: true, SHA256: exactSHA256(before), Content: before, Mode: 0o600, CheckMode: true,
		},
		After: ExactFileState{
			Exists: true, SHA256: exactSHA256(after), Mode: 0o644, CheckMode: true,
		},
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := svc.RevertFile(canceled, valid); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled revert error = %v", err)
	}
	invalid := valid
	invalid.Path = ""
	if _, err := svc.RevertFile(ctx, invalid); err == nil {
		t.Fatal("blank revert path was accepted")
	}
	invalid = valid
	invalid.TransactionID = ""
	if _, err := svc.RevertFile(ctx, invalid); err == nil {
		t.Fatal("blank revert transaction id was accepted")
	}
	invalid = valid
	invalid.Before.SHA256 = "wrong"
	if _, err := svc.RevertFile(ctx, invalid); err == nil {
		t.Fatal("inconsistent before digest was accepted")
	}
	invalid = valid
	invalid.Before = ExactFileState{Content: []byte("unexpected")}
	if _, err := svc.RevertFile(ctx, invalid); err == nil {
		t.Fatal("absent before state with content was accepted")
	}
	invalid = valid
	invalid.After.SHA256 = ""
	if _, err := svc.RevertFile(ctx, invalid); err == nil {
		t.Fatal("after state without digest was accepted")
	}
	invalid = valid
	invalid.Path = "."
	if _, err := svc.RevertFile(ctx, invalid); !errors.Is(err, ErrRefusingRoot) {
		t.Fatalf("root revert error = %v, want ErrRefusingRoot", err)
	}
	invalid = valid
	invalid.Path = "../outside.txt"
	if _, err := svc.RevertFile(ctx, invalid); !errors.Is(err, ErrPathOutsideRoot) {
		t.Fatalf("outside revert error = %v, want ErrPathOutsideRoot", err)
	}
	invalid = valid
	invalid.After.Mode = 0o600
	if _, err := svc.RevertFile(ctx, invalid); !errors.Is(err, ErrExactStateMismatch) {
		t.Fatalf("mode-mismatch revert error = %v, want ErrExactStateMismatch", err)
	}
	if err := os.Mkdir(filepath.Join(svc.Root(), "directory"), 0o755); err != nil {
		t.Fatalf("Mkdir directory: %v", err)
	}
	invalid = valid
	invalid.Path = "directory"
	if _, err := svc.RevertFile(ctx, invalid); !errors.Is(err, ErrExactRevertUnsupported) {
		t.Fatalf("directory revert error = %v, want ErrExactRevertUnsupported", err)
	}
	invalid = valid
	invalid.Path = "missing/notes.txt"
	invalid.After = ExactFileState{}
	if _, err := svc.RevertFile(ctx, invalid); err == nil {
		t.Fatal("restore into missing parent was accepted")
	}

	denied := newTestService(t, WithApproval(func(context.Context, Operation) error {
		return errors.New("denied")
	}))
	deniedPath := filepath.Join(denied.Root(), "notes.txt")
	if err := os.WriteFile(deniedPath, after, 0o644); err != nil {
		t.Fatalf("WriteFile denied fixture: %v", err)
	}
	if _, err := denied.RevertFile(ctx, valid); !errors.Is(err, ErrApprovalDenied) {
		t.Fatalf("denied revert error = %v, want ErrApprovalDenied", err)
	}
	content, _ := os.ReadFile(deniedPath)
	if string(content) != string(after) {
		t.Fatalf("denied revert changed content to %q", content)
	}

	transactionDir, _, _ := exactRevertTransactionPaths(valid.Path, valid.TransactionID)
	if err := os.Mkdir(filepath.Join(svc.Root(), transactionDir), 0o700); err != nil {
		t.Fatalf("Mkdir pending transaction: %v", err)
	}
	recovered, err := svc.RevertFile(ctx, valid)
	if err != nil {
		t.Fatalf("pending transaction revert: %v", err)
	}
	if !recovered.Changed || !recovered.Restored || recovered.Reused {
		t.Fatalf("pending transaction result = %+v", recovered)
	}
	if content, err := os.ReadFile(path); err != nil || string(content) != string(before) {
		t.Fatalf("pending transaction target = %q, error %v", content, err)
	}
	if _, err := os.Stat(filepath.Join(svc.Root(), transactionDir)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pending transaction stat = %v, want not-exist", err)
	}
}

func TestServiceRecoverPendingRevertTransactionStates(t *testing.T) {
	updateRequest := func(transactionID string) RevertFileRequest {
		return RevertFileRequest{
			Path:          "notes.txt",
			TransactionID: transactionID,
			Before: ExactFileState{
				Exists: true, SHA256: exactSHA256([]byte("before\n")), Content: []byte("before\n"), Mode: 0o600, CheckMode: true,
			},
			After: ExactFileState{
				Exists: true, SHA256: exactSHA256([]byte("after\n")), Mode: 0o644, CheckMode: true,
			},
		}
	}
	prepareQuarantine := func(t *testing.T, svc *Service, req RevertFileRequest) (*os.Root, string, string) {
		t.Helper()
		root, err := os.OpenRoot(svc.Root())
		if err != nil {
			t.Fatalf("OpenRoot: %v", err)
		}
		transactionDir, quarantine, _ := exactRevertTransactionPaths(req.Path, req.TransactionID)
		if err := root.Mkdir(transactionDir, 0o700); err != nil {
			root.Close()
			t.Fatalf("Mkdir transaction: %v", err)
		}
		if err := root.Rename(req.Path, quarantine); err != nil {
			root.Close()
			t.Fatalf("Rename quarantine: %v", err)
		}
		return root, transactionDir, quarantine
	}
	assertTransactionRemoved := func(t *testing.T, svc *Service, transactionDir string) {
		t.Helper()
		if _, err := os.Stat(filepath.Join(svc.Root(), transactionDir)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("transaction directory stat = %v, want not-exist", err)
		}
	}

	t.Run("cleans pre-quarantine replacement", func(t *testing.T) {
		svc := newTestService(t)
		req := updateRequest("pre-quarantine-update")
		if err := os.WriteFile(filepath.Join(svc.Root(), req.Path), []byte("after\n"), 0o644); err != nil {
			t.Fatalf("WriteFile after: %v", err)
		}
		root, err := os.OpenRoot(svc.Root())
		if err != nil {
			t.Fatalf("OpenRoot: %v", err)
		}
		defer root.Close()
		transactionDir, _, replacement := exactRevertTransactionPaths(req.Path, req.TransactionID)
		if err := root.Mkdir(transactionDir, 0o700); err != nil {
			t.Fatalf("Mkdir transaction: %v", err)
		}
		if err := os.WriteFile(filepath.Join(svc.Root(), replacement), []byte("before\n"), 0o600); err != nil {
			t.Fatalf("WriteFile replacement: %v", err)
		}

		if err := svc.RecoverPendingRevert(context.Background(), req); err != nil {
			t.Fatalf("RecoverPendingRevert: %v", err)
		}
		content, err := os.ReadFile(filepath.Join(svc.Root(), req.Path))
		if err != nil || string(content) != "after\n" {
			t.Fatalf("unchanged target = %q, error %v", content, err)
		}
		assertTransactionRemoved(t, svc, transactionDir)
	})

	t.Run("restores interrupted update quarantine", func(t *testing.T) {
		svc := newTestService(t)
		req := updateRequest("interrupted-update")
		if err := os.WriteFile(filepath.Join(svc.Root(), req.Path), []byte("after\n"), 0o644); err != nil {
			t.Fatalf("WriteFile after: %v", err)
		}
		root, transactionDir, _ := prepareQuarantine(t, svc, req)
		defer root.Close()

		if err := svc.RecoverPendingRevert(context.Background(), req); err != nil {
			t.Fatalf("RecoverPendingRevert: %v", err)
		}
		content, err := os.ReadFile(filepath.Join(svc.Root(), req.Path))
		if err != nil || string(content) != "after\n" {
			t.Fatalf("restored target = %q, error %v", content, err)
		}
		assertTransactionRemoved(t, svc, transactionDir)
	})

	t.Run("cleans restored update quarantine", func(t *testing.T) {
		svc := newTestService(t)
		req := updateRequest("restored-update")
		if err := os.WriteFile(filepath.Join(svc.Root(), req.Path), []byte("after\n"), 0o644); err != nil {
			t.Fatalf("WriteFile after: %v", err)
		}
		root, transactionDir, quarantine := prepareQuarantine(t, svc, req)
		defer root.Close()
		if err := root.Link(quarantine, req.Path); err != nil {
			t.Fatalf("Link restored target: %v", err)
		}

		if err := svc.RecoverPendingRevert(context.Background(), req); err != nil {
			t.Fatalf("RecoverPendingRevert: %v", err)
		}
		content, err := os.ReadFile(filepath.Join(svc.Root(), req.Path))
		if err != nil || string(content) != "after\n" {
			t.Fatalf("restored target = %q, error %v", content, err)
		}
		assertTransactionRemoved(t, svc, transactionDir)
	})

	t.Run("cleans completed update quarantine", func(t *testing.T) {
		svc := newTestService(t)
		req := updateRequest("completed-update")
		target := filepath.Join(svc.Root(), req.Path)
		if err := os.WriteFile(target, []byte("after\n"), 0o644); err != nil {
			t.Fatalf("WriteFile after: %v", err)
		}
		root, transactionDir, _ := prepareQuarantine(t, svc, req)
		defer root.Close()
		_, _, replacement := exactRevertTransactionPaths(req.Path, req.TransactionID)
		replacementFile, err := root.OpenFile(replacement, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatalf("OpenFile replacement: %v", err)
		}
		if _, err := replacementFile.Write([]byte("before\n")); err != nil {
			replacementFile.Close()
			t.Fatalf("Write replacement: %v", err)
		}
		if err := replacementFile.Close(); err != nil {
			t.Fatalf("Close replacement: %v", err)
		}
		if err := root.Link(replacement, req.Path); err != nil {
			t.Fatalf("Link installed replacement: %v", err)
		}

		if err := svc.RecoverPendingRevert(context.Background(), req); err != nil {
			t.Fatalf("RecoverPendingRevert: %v", err)
		}
		content, err := os.ReadFile(target)
		if err != nil || string(content) != "before\n" {
			t.Fatalf("completed target = %q, error %v", content, err)
		}
		assertTransactionRemoved(t, svc, transactionDir)
	})

	t.Run("cleans completed create revert quarantine", func(t *testing.T) {
		svc := newTestService(t)
		after := []byte("created\n")
		req := RevertFileRequest{
			Path:          "created.txt",
			TransactionID: "completed-create",
			Before:        ExactFileState{},
			After: ExactFileState{
				Exists: true, SHA256: exactSHA256(after), Mode: 0o640, CheckMode: true,
			},
		}
		if err := os.WriteFile(filepath.Join(svc.Root(), req.Path), after, 0o640); err != nil {
			t.Fatalf("WriteFile created: %v", err)
		}
		root, transactionDir, _ := prepareQuarantine(t, svc, req)
		defer root.Close()

		if err := svc.RecoverPendingRevert(context.Background(), req); err != nil {
			t.Fatalf("RecoverPendingRevert: %v", err)
		}
		if _, err := os.Stat(filepath.Join(svc.Root(), req.Path)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("created target stat = %v, want not-exist", err)
		}
		assertTransactionRemoved(t, svc, transactionDir)
	})

	t.Run("preserves conflicting target and quarantine", func(t *testing.T) {
		svc := newTestService(t)
		req := updateRequest("conflicting-update")
		target := filepath.Join(svc.Root(), req.Path)
		if err := os.WriteFile(target, []byte("after\n"), 0o644); err != nil {
			t.Fatalf("WriteFile after: %v", err)
		}
		root, transactionDir, quarantine := prepareQuarantine(t, svc, req)
		defer root.Close()
		if err := os.WriteFile(target, []byte("external\n"), 0o644); err != nil {
			t.Fatalf("WriteFile external: %v", err)
		}

		if err := svc.RecoverPendingRevert(context.Background(), req); !errors.Is(err, ErrExactRevertPending) {
			t.Fatalf("conflicting recovery error = %v, want ErrExactRevertPending", err)
		}
		if content, _ := os.ReadFile(target); string(content) != "external\n" {
			t.Fatalf("conflicting target changed to %q", content)
		}
		if content, err := root.ReadFile(quarantine); err != nil || string(content) != "after\n" {
			t.Fatalf("preserved quarantine = %q, error %v", content, err)
		}
		if _, err := os.Stat(filepath.Join(svc.Root(), transactionDir)); err != nil {
			t.Fatalf("preserved transaction stat: %v", err)
		}
	})
}

func TestServiceRecoverPendingRevertRejectsMalformedAndUnsafeState(t *testing.T) {
	ctx := context.Background()
	var nilService *Service
	if err := nilService.RecoverPendingRevert(ctx, RevertFileRequest{}); err == nil {
		t.Fatal("nil service recovered a transaction")
	}
	svc := newTestService(t)
	after := []byte("after\n")
	before := []byte("before\n")
	if err := os.WriteFile(filepath.Join(svc.Root(), "notes.txt"), after, 0o644); err != nil {
		t.Fatalf("WriteFile after: %v", err)
	}
	valid := RevertFileRequest{
		Path:          "notes.txt",
		TransactionID: "recover-validation",
		Before: ExactFileState{
			Exists: true, SHA256: exactSHA256(before), Content: before, Mode: 0o600, CheckMode: true,
		},
		After: ExactFileState{
			Exists: true, SHA256: exactSHA256(after), Mode: 0o644, CheckMode: true,
		},
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := svc.RecoverPendingRevert(canceled, valid); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled recovery error = %v, want context.Canceled", err)
	}
	tests := []struct {
		name   string
		mutate func(*RevertFileRequest)
	}{
		{"blank path", func(req *RevertFileRequest) { req.Path = "" }},
		{"blank transaction", func(req *RevertFileRequest) { req.TransactionID = "" }},
		{"before digest", func(req *RevertFileRequest) { req.Before.SHA256 = "wrong" }},
		{"absent before content", func(req *RevertFileRequest) {
			req.Before = ExactFileState{Content: []byte("unexpected")}
		}},
		{"after digest", func(req *RevertFileRequest) { req.After.SHA256 = "" }},
		{"outside path", func(req *RevertFileRequest) { req.Path = "../outside.txt" }},
		{"workspace root", func(req *RevertFileRequest) { req.Path = "." }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := valid
			test.mutate(&req)
			if err := svc.RecoverPendingRevert(ctx, req); err == nil {
				t.Fatal("unsafe recovery state was accepted")
			}
		})
	}

	t.Run("denied recovery preserves transaction", func(t *testing.T) {
		denied := newTestService(t, WithApproval(func(context.Context, Operation) error {
			return errors.New("denied")
		}))
		req := valid
		req.TransactionID = "denied-recovery"
		if err := os.WriteFile(filepath.Join(denied.Root(), req.Path), after, 0o644); err != nil {
			t.Fatalf("WriteFile denied target: %v", err)
		}
		transactionDir, _, replacement := exactRevertTransactionPaths(req.Path, req.TransactionID)
		if err := os.Mkdir(filepath.Join(denied.Root(), transactionDir), 0o700); err != nil {
			t.Fatalf("Mkdir denied transaction: %v", err)
		}
		replacementPath := filepath.Join(denied.Root(), replacement)
		if err := os.WriteFile(replacementPath, before, 0o600); err != nil {
			t.Fatalf("WriteFile denied replacement: %v", err)
		}
		if err := denied.RecoverPendingRevert(ctx, req); !errors.Is(err, ErrApprovalDenied) {
			t.Fatalf("denied recovery error = %v, want ErrApprovalDenied", err)
		}
		if content, err := os.ReadFile(replacementPath); err != nil || string(content) != string(before) {
			t.Fatalf("denied replacement = %q, error %v", content, err)
		}
	})

	t.Run("transaction path is not a directory", func(t *testing.T) {
		req := valid
		req.TransactionID = "regular-transaction"
		transactionDir, _, _ := exactRevertTransactionPaths(req.Path, req.TransactionID)
		if err := os.WriteFile(filepath.Join(svc.Root(), transactionDir), []byte("occupied"), 0o600); err != nil {
			t.Fatalf("WriteFile transaction path: %v", err)
		}
		if err := svc.RecoverPendingRevert(ctx, req); !errors.Is(err, ErrExactRevertPending) {
			t.Fatalf("regular transaction error = %v, want ErrExactRevertPending", err)
		}
	})

	t.Run("replacement is not regular", func(t *testing.T) {
		req := valid
		req.TransactionID = "directory-replacement"
		transactionDir, _, replacement := exactRevertTransactionPaths(req.Path, req.TransactionID)
		if err := os.Mkdir(filepath.Join(svc.Root(), transactionDir), 0o700); err != nil {
			t.Fatalf("Mkdir transaction: %v", err)
		}
		if err := os.Mkdir(filepath.Join(svc.Root(), replacement), 0o700); err != nil {
			t.Fatalf("Mkdir replacement: %v", err)
		}
		if err := svc.RecoverPendingRevert(ctx, req); !errors.Is(err, ErrExactRevertPending) {
			t.Fatalf("directory replacement error = %v, want ErrExactRevertPending", err)
		}
	})

	t.Run("quarantine digest is inconsistent", func(t *testing.T) {
		req := valid
		req.TransactionID = "bad-quarantine"
		transactionDir, quarantine, _ := exactRevertTransactionPaths(req.Path, req.TransactionID)
		if err := os.Mkdir(filepath.Join(svc.Root(), transactionDir), 0o700); err != nil {
			t.Fatalf("Mkdir transaction: %v", err)
		}
		if err := os.WriteFile(filepath.Join(svc.Root(), quarantine), []byte("unexpected"), 0o644); err != nil {
			t.Fatalf("WriteFile quarantine: %v", err)
		}
		if err := svc.RecoverPendingRevert(ctx, req); !errors.Is(err, ErrExactRevertPending) {
			t.Fatalf("bad quarantine error = %v, want ErrExactRevertPending", err)
		}
	})

	t.Run("quarantine is impossible for absent after state", func(t *testing.T) {
		req := valid
		req.TransactionID = "absent-after-quarantine"
		req.After = ExactFileState{}
		transactionDir, quarantine, _ := exactRevertTransactionPaths(req.Path, req.TransactionID)
		if err := os.Mkdir(filepath.Join(svc.Root(), transactionDir), 0o700); err != nil {
			t.Fatalf("Mkdir transaction: %v", err)
		}
		if err := os.WriteFile(filepath.Join(svc.Root(), quarantine), after, 0o644); err != nil {
			t.Fatalf("WriteFile quarantine: %v", err)
		}
		if err := svc.RecoverPendingRevert(ctx, req); !errors.Is(err, ErrExactRevertPending) {
			t.Fatalf("absent-after quarantine error = %v, want ErrExactRevertPending", err)
		}
	})
}

func TestExactRootFileHelpersRejectMismatchesAndCleanTemporaryFiles(t *testing.T) {
	mode := iofs.FileMode(0o640) | iofs.ModeSetuid | iofs.ModeSetgid | iofs.ModeSticky | iofs.ModeDir
	wantMode := iofs.FileMode(0o640) | iofs.ModeSetuid | iofs.ModeSetgid | iofs.ModeSticky
	if got := exactFileMode(mode); got != wantMode {
		t.Fatalf("exactFileMode(%v) = %v, want %v", mode, got, wantMode)
	}
	rootPath := t.TempDir()
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	defer root.Close()
	if err := verifyExactRootFileState(root, "missing.txt", ExactFileState{}); err != nil {
		t.Fatalf("absent exact state: %v", err)
	}
	if err := verifyExactRootFileState(root, "missing.txt", ExactFileState{Exists: true}); !errors.Is(err, ErrExactStateMismatch) {
		t.Fatalf("missing expected file error = %v", err)
	}
	if err := os.Mkdir(filepath.Join(rootPath, "dir"), 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := verifyExactRootFileState(root, "dir", ExactFileState{Exists: true}); !errors.Is(err, ErrExactRevertUnsupported) {
		t.Fatalf("directory exact state error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(rootPath, "notes.txt"), []byte("after"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := verifyExactFileState(filepath.Join(rootPath, "missing.txt"), ExactFileState{Exists: true}); !errors.Is(err, ErrExactStateMismatch) {
		t.Fatalf("missing unrooted exact state error = %v", err)
	}
	if err := verifyExactFileState(filepath.Join(rootPath, "notes.txt"), ExactFileState{}); !errors.Is(err, ErrExactStateMismatch) {
		t.Fatalf("unexpected existing unrooted file error = %v", err)
	}
	if err := os.Symlink("notes.txt", filepath.Join(rootPath, "linked.txt")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	if err := verifyExactFileState(filepath.Join(rootPath, "linked.txt"), ExactFileState{Exists: true}); !errors.Is(err, ErrExactRevertSymlink) {
		t.Fatalf("unrooted symlink exact state error = %v", err)
	}
	if err := rejectSymlinkComponents(rootPath, filepath.Join(rootPath, "missing", "file.txt")); err != nil {
		t.Fatalf("missing unrooted path components: %v", err)
	}
	if err := rejectSymlinkComponents(rootPath, rootPath); err != nil {
		t.Fatalf("root unrooted path component: %v", err)
	}
	if err := rejectRootSymlinkComponents(root, "missing/file.txt"); err != nil {
		t.Fatalf("missing rooted path components: %v", err)
	}
	if err := rejectRootSymlinkComponents(root, "."); err != nil {
		t.Fatalf("root rooted path component: %v", err)
	}
	if err := rejectRootSymlinkComponents(root, "linked.txt"); !errors.Is(err, ErrExactRevertSymlink) {
		t.Fatalf("rooted symlink component error = %v", err)
	}
	if err := verifyExactRootFileState(root, "linked.txt", ExactFileState{Exists: true}); !errors.Is(err, ErrExactRevertSymlink) {
		t.Fatalf("rooted symlink exact state error = %v", err)
	}
	if err := verifyExactRootFileState(root, "notes.txt", ExactFileState{}); !errors.Is(err, ErrExactStateMismatch) {
		t.Fatalf("unexpected existing file error = %v", err)
	}
	if err := verifyExactRootFileState(root, "notes.txt", ExactFileState{
		Exists: true, SHA256: exactSHA256([]byte("wrong")),
	}); !errors.Is(err, ErrExactStateMismatch) {
		t.Fatalf("digest mismatch error = %v", err)
	}
	if err := verifyExactRootFileState(root, "notes.txt", ExactFileState{
		Exists: true, SHA256: exactSHA256([]byte("after")), Mode: 0o600, CheckMode: true,
	}); !errors.Is(err, ErrExactStateMismatch) {
		t.Fatalf("mode mismatch error = %v", err)
	}
	if err := atomicWriteRootFile(root, "notes.txt", []byte("before"), 0o600, ExactFileState{
		Exists: true, SHA256: exactSHA256([]byte("stale")),
	}, "stale-write"); !errors.Is(err, ErrExactStateMismatch) {
		t.Fatalf("stale atomic write error = %v", err)
	}
	content, err := os.ReadFile(filepath.Join(rootPath, "notes.txt"))
	if err != nil || string(content) != "after" {
		t.Fatalf("stale atomic write changed file to %q, error %v", content, err)
	}
	if err := atomicWriteRootFile(root, "notes.txt", []byte("before"), 0o600, ExactFileState{}, "occupied-write"); !errors.Is(err, ErrExactStateMismatch) {
		t.Fatalf("unexpected-destination atomic write error = %v", err)
	}
	if err := atomicWriteRootFile(root, "missing/notes.txt", []byte("before"), 0o600, ExactFileState{}, "missing-parent"); err == nil {
		t.Fatal("atomic write into missing parent succeeded")
	}
	if err := removeExactRootFile(root, "missing.txt", ExactFileState{Exists: true, SHA256: "missing"}, "missing-remove"); err == nil {
		t.Fatal("exact remove of missing file succeeded")
	}
	if _, _, err := quarantineExactRootFile(root, "missing.txt", ExactFileState{Exists: true, SHA256: "missing"}, "missing-quarantine"); err == nil {
		t.Fatal("quarantine of missing file succeeded")
	}

	if err := os.WriteFile(filepath.Join(rootPath, "quarantine.txt"), []byte("quarantine"), 0o600); err != nil {
		t.Fatalf("WriteFile quarantine: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rootPath, "occupied.txt"), []byte("occupied"), 0o600); err != nil {
		t.Fatalf("WriteFile occupied: %v", err)
	}
	if err := restoreQuarantinedRootFile(root, "quarantine.txt", "occupied.txt"); err == nil {
		t.Fatal("quarantine restore overwrote occupied destination")
	}
	if got, _ := os.ReadFile(filepath.Join(rootPath, "occupied.txt")); string(got) != "occupied" {
		t.Fatalf("occupied destination changed to %q", got)
	}
	if err := os.WriteFile(filepath.Join(rootPath, "not-a-directory"), []byte("file"), 0o600); err != nil {
		t.Fatalf("WriteFile regular parent: %v", err)
	}
	if _, err := unusedRootPath(root, "not-a-directory", ".gollem-revert-"); err == nil {
		t.Fatal("temporary path below regular file was allocated")
	}
	if err := os.Mkdir(filepath.Join(rootPath, "concurrent-dir"), 0o755); err != nil {
		t.Fatalf("Mkdir concurrent directory: %v", err)
	}
	if _, _, err := quarantineExactRootFile(root, "concurrent-dir", ExactFileState{
		Exists: true, SHA256: exactSHA256([]byte("expected file")),
	}, "concurrent-directory"); !errors.Is(err, ErrExactRevertUnsupported) {
		t.Fatalf("concurrent-directory quarantine error = %v, want ErrExactRevertUnsupported", err)
	}
	entries, err := os.ReadDir(rootPath)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	preservedConcurrentDirectories := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".gollem-revert-") {
			if entry.IsDir() {
				preservedConcurrentDirectories++
				continue
			}
			t.Fatalf("stale atomic write leaked temp file %q", entry.Name())
		}
	}
	if preservedConcurrentDirectories != 1 {
		t.Fatalf("preserved concurrent directories = %d, want 1", preservedConcurrentDirectories)
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

func TestServiceMutationOptions(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	if err := svc.CreateDirectoryWithOptions(ctx, "missing/child", CreateDirectoryOptions{Recursive: false}); err == nil {
		t.Fatal("non-recursive create with a missing parent succeeded")
	}
	if _, err := os.Stat(filepath.Join(svc.Root(), "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing parent stat error = %v, want not exist", err)
	}
	if err := svc.CreateDirectory(ctx, "parent"); err != nil {
		t.Fatalf("CreateDirectory parent: %v", err)
	}
	if err := svc.CreateDirectoryWithOptions(ctx, "parent/child", CreateDirectoryOptions{Recursive: false}); err != nil {
		t.Fatalf("non-recursive CreateDirectory: %v", err)
	}

	if err := svc.RemoveWithOptions(ctx, "absent", RemoveOptions{Recursive: true, Force: false}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("non-force missing remove error = %v, want not exist", err)
	}
	if err := svc.RemoveWithOptions(ctx, "absent", RemoveOptions{Recursive: true, Force: true}); err != nil {
		t.Fatalf("force missing remove: %v", err)
	}
	if err := svc.WriteFile(ctx, "populated/file.txt", []byte("keep"), 0); err != nil {
		t.Fatalf("WriteFile populated: %v", err)
	}
	if err := svc.RemoveWithOptions(ctx, "populated", RemoveOptions{Recursive: false, Force: true}); err == nil {
		t.Fatal("non-recursive populated directory remove succeeded")
	}
	if _, err := os.Stat(filepath.Join(svc.Root(), "populated/file.txt")); err != nil {
		t.Fatalf("file removed by failed non-recursive remove: %v", err)
	}
	if err := svc.CreateDirectory(ctx, "empty"); err != nil {
		t.Fatalf("CreateDirectory empty: %v", err)
	}
	if err := svc.RemoveWithOptions(ctx, "empty", RemoveOptions{Recursive: false, Force: false}); err != nil {
		t.Fatalf("non-recursive empty directory remove: %v", err)
	}
	if err := svc.WriteFile(ctx, "single.txt", []byte("single"), 0); err != nil {
		t.Fatalf("WriteFile single: %v", err)
	}
	if err := svc.RemoveWithOptions(ctx, "single.txt", RemoveOptions{Recursive: false, Force: false}); err != nil {
		t.Fatalf("non-recursive file remove: %v", err)
	}

	if err := svc.WriteFile(ctx, "tree/nested/file.txt", []byte("copy"), 0); err != nil {
		t.Fatalf("WriteFile tree: %v", err)
	}
	if err := svc.CopyWithOptions(ctx, "tree", "tree-flat", CopyOptions{Recursive: false}); !errors.Is(err, ErrRecursiveRequired) {
		t.Fatalf("non-recursive directory copy error = %v, want ErrRecursiveRequired", err)
	}
	if _, err := os.Stat(filepath.Join(svc.Root(), "tree-flat")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("non-recursive copy destination stat error = %v, want not exist", err)
	}
	if err := svc.CopyWithOptions(ctx, "tree/nested/file.txt", "file-copy.txt", CopyOptions{Recursive: false}); err != nil {
		t.Fatalf("non-recursive file copy: %v", err)
	}
	if err := svc.CopyWithOptions(ctx, "tree", "tree-copy", CopyOptions{Recursive: true}); err != nil {
		t.Fatalf("recursive directory copy: %v", err)
	}
}

func TestServiceMutationOptionsUseApprovalAndAuditPath(t *testing.T) {
	ctx := context.Background()
	var approved []Operation
	var audited []AuditEvent
	svc := newTestService(t,
		WithApproval(func(_ context.Context, operation Operation) error {
			approved = append(approved, operation)
			return nil
		}),
		WithAuditSink(func(event AuditEvent) {
			audited = append(audited, event)
		}),
	)
	if err := os.Mkdir(filepath.Join(svc.Root(), "parent"), 0o755); err != nil {
		t.Fatalf("Mkdir parent: %v", err)
	}
	if err := os.WriteFile(filepath.Join(svc.Root(), "source.txt"), []byte("source"), 0o644); err != nil {
		t.Fatalf("WriteFile source: %v", err)
	}

	if err := svc.CreateDirectoryWithOptions(ctx, "parent/child", CreateDirectoryOptions{Recursive: false}); err != nil {
		t.Fatalf("CreateDirectoryWithOptions: %v", err)
	}
	if err := svc.CopyWithOptions(ctx, "source.txt", "copy.txt", CopyOptions{Recursive: false}); err != nil {
		t.Fatalf("CopyWithOptions: %v", err)
	}
	if err := svc.RemoveWithOptions(ctx, "copy.txt", RemoveOptions{Recursive: false, Force: false}); err != nil {
		t.Fatalf("RemoveWithOptions: %v", err)
	}

	want := []OperationKind{OperationCreateDirectory, OperationCopy, OperationRemove}
	if len(approved) != len(want) || len(audited) != len(want) {
		t.Fatalf("approved/audited counts = %d/%d, want %d/%d", len(approved), len(audited), len(want), len(want))
	}
	for index, kind := range want {
		if approved[index].Kind != kind || audited[index].Operation.Kind != kind || !audited[index].Allowed {
			t.Fatalf("mutation %d approved/audited = %+v/%+v, want %s allowed", index, approved[index], audited[index], kind)
		}
	}
}

func TestServiceMetadataReportsInternalSymlinkTargetKind(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	if err := os.WriteFile(filepath.Join(svc.Root(), "target.txt"), []byte("target"), 0o644); err != nil {
		t.Fatalf("WriteFile target: %v", err)
	}
	if err := os.Symlink("target.txt", filepath.Join(svc.Root(), "link.txt")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	metadata, err := svc.Metadata(ctx, "link.txt")
	if err != nil {
		t.Fatalf("Metadata symlink: %v", err)
	}
	if metadata.IsDir || !metadata.IsFile || !metadata.IsSymlink {
		t.Fatalf("symlink metadata = %+v", metadata)
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

func TestExactRootReplacementRestoresQuarantineOnFailure(t *testing.T) {
	t.Run("successful install removes quarantine", func(t *testing.T) {
		ops := &scriptedExactRootOps{}
		if err := installRootReplacement(ops, "temp", "notes.txt", "quarantine"); err != nil {
			t.Fatalf("install replacement: %v", err)
		}
		if got := strings.Join(ops.calls, ","); got != "link temp notes.txt,remove quarantine" {
			t.Fatalf("successful replacement calls = %q", got)
		}
	})

	t.Run("failed install restores original", func(t *testing.T) {
		ops := &scriptedExactRootOps{
			linkErrors: []error{errors.New("install failed"), nil},
		}
		if err := installRootReplacement(ops, "temp", "notes.txt", "quarantine"); err == nil {
			t.Fatal("failed installation returned nil")
		}
		if got := strings.Join(ops.calls, ","); got != "link temp notes.txt,link quarantine notes.txt,remove quarantine" {
			t.Fatalf("replacement calls = %q", got)
		}
	})

	t.Run("occupied destination preserves quarantine", func(t *testing.T) {
		ops := &scriptedExactRootOps{
			linkErrors: []error{errors.New("install failed"), errors.New("destination occupied")},
		}
		if err := installRootReplacement(ops, "temp", "notes.txt", "quarantine"); err == nil ||
			!strings.Contains(err.Error(), `preserved at "quarantine"`) {
			t.Fatalf("occupied replacement error = %v", err)
		}
		if got := strings.Join(ops.calls, ","); got != "link temp notes.txt,link quarantine notes.txt" {
			t.Fatalf("occupied replacement calls = %q", got)
		}
	})

	t.Run("failed cleanup rolls back installed file", func(t *testing.T) {
		ops := &scriptedExactRootOps{
			removeErrors: []error{errors.New("cleanup failed"), nil, nil},
		}
		if err := installRootReplacement(ops, "temp", "notes.txt", "quarantine"); err == nil {
			t.Fatal("failed cleanup returned nil")
		}
		want := "link temp notes.txt,remove quarantine,remove notes.txt,link quarantine notes.txt,remove quarantine"
		if got := strings.Join(ops.calls, ","); got != want {
			t.Fatalf("cleanup rollback calls = %q, want %q", got, want)
		}
	})

	t.Run("failed cleanup preserves prior quarantine when installed file cannot be removed", func(t *testing.T) {
		ops := &scriptedExactRootOps{
			removeErrors: []error{errors.New("cleanup failed"), errors.New("installed file busy")},
		}
		if err := installRootReplacement(ops, "temp", "notes.txt", "quarantine"); err == nil ||
			!strings.Contains(err.Error(), `preserved prior file at "quarantine"`) {
			t.Fatalf("cleanup preservation error = %v", err)
		}
		want := "link temp notes.txt,remove quarantine,remove notes.txt"
		if got := strings.Join(ops.calls, ","); got != want {
			t.Fatalf("cleanup preservation calls = %q, want %q", got, want)
		}
	})

	t.Run("failed cleanup reports failed restoration", func(t *testing.T) {
		ops := &scriptedExactRootOps{
			linkErrors:   []error{nil, errors.New("restore failed")},
			removeErrors: []error{errors.New("cleanup failed"), nil},
		}
		if err := installRootReplacement(ops, "temp", "notes.txt", "quarantine"); err == nil ||
			!strings.Contains(err.Error(), `preserved at "quarantine"`) {
			t.Fatalf("cleanup restore error = %v", err)
		}
		want := "link temp notes.txt,remove quarantine,remove notes.txt,link quarantine notes.txt"
		if got := strings.Join(ops.calls, ","); got != want {
			t.Fatalf("cleanup restore calls = %q, want %q", got, want)
		}
	})

	t.Run("failed quarantine removal restores original", func(t *testing.T) {
		ops := &scriptedExactRootOps{
			removeErrors: []error{errors.New("remove failed"), nil},
		}
		if err := removeQuarantinedRootFile(ops, "quarantine", "notes.txt"); err == nil {
			t.Fatal("failed quarantine removal returned nil")
		}
		if got := strings.Join(ops.calls, ","); got != "remove quarantine,link quarantine notes.txt,remove quarantine" {
			t.Fatalf("removal recovery calls = %q", got)
		}
	})

	t.Run("failed quarantine removal preserves bytes when restoration fails", func(t *testing.T) {
		ops := &scriptedExactRootOps{
			linkErrors:   []error{errors.New("restore occupied")},
			removeErrors: []error{errors.New("remove failed")},
		}
		if err := removeQuarantinedRootFile(ops, "quarantine", "notes.txt"); err == nil ||
			!strings.Contains(err.Error(), `preserved at "quarantine"`) {
			t.Fatalf("failed removal restoration error = %v", err)
		}
		want := "remove quarantine,link quarantine notes.txt"
		if got := strings.Join(ops.calls, ","); got != want {
			t.Fatalf("failed removal restoration calls = %q, want %q", got, want)
		}
	})

	t.Run("failed restored-link cleanup reports the preserved link", func(t *testing.T) {
		ops := &scriptedExactRootOps{
			removeErrors: []error{errors.New("cleanup failed")},
		}
		if err := restoreQuarantinedRootFile(ops, "quarantine", "notes.txt"); err == nil ||
			!strings.Contains(err.Error(), "remove restored quarantine link") {
			t.Fatalf("restored-link cleanup error = %v", err)
		}
		if got := strings.Join(ops.calls, ","); got != "link quarantine notes.txt,remove quarantine" {
			t.Fatalf("restored-link cleanup calls = %q", got)
		}
	})
}

type scriptedExactRootOps struct {
	linkErrors   []error
	removeErrors []error
	calls        []string
}

func (s *scriptedExactRootOps) Link(oldname, newname string) error {
	s.calls = append(s.calls, "link "+oldname+" "+newname)
	if len(s.linkErrors) == 0 {
		return nil
	}
	err := s.linkErrors[0]
	s.linkErrors = s.linkErrors[1:]
	return err
}

func (s *scriptedExactRootOps) Remove(name string) error {
	s.calls = append(s.calls, "remove "+name)
	if len(s.removeErrors) == 0 {
		return nil
	}
	err := s.removeErrors[0]
	s.removeErrors = s.removeErrors[1:]
	return err
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
