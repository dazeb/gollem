package codetool

// Regression tests for edit/multi_edit/write bug fixes:
//   - panic guards for whitespace-only / all-blank old_string on empty files
//   - mixed line-ending preservation
//   - multi_edit empty-string sentinel discarding valid auto-corrections
//   - chmod auto-upgrade when overwriting an existing script
//   - edit success context anchoring to the actual edit site

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/fugue-labs/gollem/core"
)

// TestEdit_BlankOldStringNoPanic covers the autoCorrectWhitespace and
// autoCorrectInternalBlankLines panics: editing an empty or whitespace-only
// file with an old_string that normalizes to "" used to hit
// strings.Index(s, "") == 0 followed by an out-of-range [idx+1:] slice.
// Both edit and multi_edit must return a retry error instead of panicking.
func TestEdit_BlankOldStringNoPanic(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		oldString string
	}{
		{"empty file, space old_string", "", " "},
		{"empty file, tab old_string", "", "\t"},
		{"whitespace-only file, tab old_string", "   ", "\t"},
		{"empty file, blank line old_string", "", "\n"},
		{"empty file, two blank lines old_string", "", "\n\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "f.txt")
			if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
				t.Fatal(err)
			}

			editTool := Edit(WithWorkDir(dir))
			args := fmt.Sprintf(`{"path": "f.txt", "old_string": %q, "new_string": "x"}`, tt.oldString)
			err := callErr(t, editTool, args)
			if err == nil {
				t.Fatal("edit: expected error, got success")
			}
			var retry *core.ModelRetryError
			if !errors.As(err, &retry) {
				t.Errorf("edit: expected ModelRetryError, got %T: %v", err, err)
			}

			multiTool := MultiEdit(WithWorkDir(dir))
			margs := fmt.Sprintf(`{"edits": [{"path": "f.txt", "old_string": %q, "new_string": "x"}]}`, tt.oldString)
			err = callErr(t, multiTool, margs)
			if err == nil {
				t.Fatal("multi_edit: expected error, got success")
			}
			retry = nil
			if !errors.As(err, &retry) {
				t.Errorf("multi_edit: expected ModelRetryError, got %T: %v", err, err)
			}

			// The file must be left unmodified.
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != tt.content {
				t.Errorf("file modified: got %q, want %q", data, tt.content)
			}
		})
	}
}

// TestAutoCorrectHelpers_EmptyNormalizedOldString exercises the exact panic
// paths at the helper level: a needle that normalizes to "" against content
// that normalizes to "" used to panic with slice bounds out of range [1:0].
func TestAutoCorrectHelpers_EmptyNormalizedOldString(t *testing.T) {
	if _, _, ok := autoCorrectWhitespace("", " ", "x"); ok {
		t.Error("autoCorrectWhitespace: whitespace-only old_string on empty content must not match")
	}
	if _, _, ok := autoCorrectWhitespace("   ", "\t", "x"); ok {
		t.Error("autoCorrectWhitespace: whitespace-only old_string on whitespace-only content must not match")
	}
	if _, _, ok := autoCorrectInternalBlankLines("", "\n", "x"); ok {
		t.Error("autoCorrectInternalBlankLines: blank old_string on empty content must not match")
	}
	if _, _, ok := autoCorrectInternalBlankLines("", "\n\n", "x"); ok {
		t.Error("autoCorrectInternalBlankLines: all-blank old_string on empty content must not match")
	}
}

// TestEdit_MixedLineEndingsPreserved verifies that editing one line of a
// mixed-ending file leaves the line endings of untouched lines byte-for-byte
// intact. Previously a single CRLF anywhere caused EVERY LF to be rewritten
// to CRLF on write-back.
func TestEdit_MixedLineEndingsPreserved(t *testing.T) {
	dir := t.TempDir()
	mixed := "lineA\nlineB\r\nlineC\n"
	path := filepath.Join(dir, "mixed.txt")
	if err := os.WriteFile(path, []byte(mixed), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := Edit(WithWorkDir(dir))
	result := call(t, tool, `{"path": "mixed.txt", "old_string": "lineC", "new_string": "lineC2"}`)
	assertContains(t, result, "Replaced 1")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "lineA\nlineB\r\nlineC2\n"
	if string(data) != want {
		t.Errorf("mixed-endings file corrupted: got %q, want %q", data, want)
	}
}

// TestMultiEdit_MixedLineEndingsPreserved is the multi_edit variant of the
// mixed line-ending regression.
func TestMultiEdit_MixedLineEndingsPreserved(t *testing.T) {
	dir := t.TempDir()
	mixed := "lineA\nlineB\r\nlineC\n"
	path := filepath.Join(dir, "mixed.txt")
	if err := os.WriteFile(path, []byte(mixed), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := MultiEdit(WithWorkDir(dir))
	result := call(t, tool, `{"edits": [{"path": "mixed.txt", "old_string": "lineC", "new_string": "lineC2"}]}`)
	assertContains(t, result, "Replaced 1")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "lineA\nlineB\r\nlineC2\n"
	if string(data) != want {
		t.Errorf("mixed-endings file corrupted: got %q, want %q", data, want)
	}
}

// TestMultiEdit_WhitespaceCorrectedDeletionOfEntireContent verifies that an
// auto-correction whose result is empty content is honored. The old
// newContent == "" sentinel discarded the successful whitespace correction
// and reported "old_string not found", diverging from the single edit tool.
func TestMultiEdit_WhitespaceCorrectedDeletionOfEntireContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	// Double space in the file, single space in old_string — triggers the
	// whitespace auto-correction; new_string "" deletes the whole content.
	if err := os.WriteFile(path, []byte("hello  world"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := MultiEdit(WithWorkDir(dir))
	result := call(t, tool, `{"edits": [{"path": "f.txt", "old_string": "hello world", "new_string": ""}]}`)
	assertContains(t, result, "auto-corrected whitespace mismatch")
	assertContains(t, result, "Replaced 1")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 0 {
		t.Errorf("expected file to be emptied, got %q", data)
	}
}

// TestWrite_UpgradesExistingScriptToExecutable verifies the documented script
// auto-upgrade actually lands for existing files. os.WriteFile only applies
// perm on creation, so overwriting a 0644 script previously left it
// non-executable despite the upgrade being computed.
func TestWrite_UpgradesExistingScriptToExecutable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run.sh")
	if err := os.WriteFile(path, []byte("echo old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := Write(WithWorkDir(dir))
	call(t, tool, `{"path": "run.sh", "content": "#!/bin/sh\necho hi\n"}`)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm&0o111 == 0 {
		t.Errorf("expected existing script to be upgraded to executable, got %o", perm)
	}

	// A non-script overwrite must keep its existing permissions untouched.
	dataPath := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(dataPath, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	call(t, tool, `{"path": "data.txt", "content": "new"}`)
	info, err = os.Stat(dataPath)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("expected non-script permissions preserved as 600, got %o", perm)
	}
}

// dupLineContent has text at line 1 that is identical to the new_string used
// to replace line 10 — the success context must anchor to line 10 (the edit
// site), not the pre-existing occurrence at line 1.
const dupLineContent = "x = compute()\n" +
	"b := 2\n" +
	"c := 3\n" +
	"d := 4\n" +
	"e := 5\n" +
	"f := 6\n" +
	"g := 7\n" +
	"h := 8\n" +
	"i := 9\n" +
	"y = old()\n" +
	"k := 11\n" +
	"l := 12\n" +
	"m := 13\n"

// TestEdit_ContextAnchorsToActualEditSite verifies the success context shows
// the replacement location instead of the first occurrence of new_string.
func TestEdit_ContextAnchorsToActualEditSite(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "dup.txt"), []byte(dupLineContent), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := Edit(WithWorkDir(dir))
	result := call(t, tool, `{"path": "dup.txt", "old_string": "y = old()", "new_string": "x = compute()"}`)
	assertContains(t, result, "Context:")
	assertContains(t, result, "    10\tx = compute()")
	assertNotContains(t, result, "     1\tx = compute()")
}

// TestMultiEdit_ContextAnchorsToActualEditSite is the multi_edit variant of
// the context-anchoring regression.
func TestMultiEdit_ContextAnchorsToActualEditSite(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "dup.txt"), []byte(dupLineContent), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := MultiEdit(WithWorkDir(dir))
	result := call(t, tool, `{"edits": [{"path": "dup.txt", "old_string": "y = old()", "new_string": "x = compute()"}]}`)
	assertContains(t, result, "Context:")
	assertContains(t, result, "    10\tx = compute()")
	assertNotContains(t, result, "     1\tx = compute()")
}
