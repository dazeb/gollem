package codetool

// Regression tests for search-tool bugs:
//   - grep/glob pruning an explicitly requested search root whose basename
//     is a skippable directory name (vendor, build, dist, ...)
//   - glob's non-** branch interpreting glob metacharacters in the search
//     path itself (e.g. Next.js "[slug]" route directories)
//   - expandBraces only expanding the first brace group
//   - ls emitting duplicate truncation markers and exceeding the 500-entry cap

import (
	"fmt"
	"strings"
	"testing"
)

// TestGrep_SkippableNamedSearchRoot verifies that grep searches a directory
// the caller explicitly asked for, even when its basename is in the skip
// list (e.g. path="vendor"), while still pruning skippable subdirectories
// during recursion from other roots.
func TestGrep_SkippableNamedSearchRoot(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "vendor/dep.go", "package dep\n\nfunc FindMeInVendor() {}\n")

	tool := Grep(WithWorkDir(dir))

	// Explicitly requested skippable-named root must be searched.
	result := call(t, tool, `{"pattern": "FindMeInVendor", "path": "vendor"}`)
	assertContains(t, result, "dep.go")
	assertContains(t, result, "FindMeInVendor")

	// But vendor/ must still be pruned when searching from the parent.
	result = call(t, tool, `{"pattern": "FindMeInVendor"}`)
	assertContains(t, result, "No matches found")
}

// TestGlob_SkippableNamedSearchRoot verifies the ** branch of glob does not
// prune the explicitly requested search root (e.g. path="vendor"), while
// skippable directories beneath the root are still pruned.
func TestGlob_SkippableNamedSearchRoot(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "vendor/a.go", "package a\n")
	writeTestFile(t, dir, "vendor/node_modules/b.go", "package b\n")

	tool := Glob(WithWorkDir(dir))
	result := call(t, tool, `{"pattern": "**/*.go", "path": "vendor"}`)
	assertContains(t, result, "a.go")
	assertNotContains(t, result, "b.go") // nested skippable dir still pruned
}

// TestGlob_MetacharactersInSearchPath verifies the non-** branch treats the
// search path literally instead of passing it through filepath.Glob, where
// "[slug]" becomes a character class and an unbalanced bracket is an error.
func TestGlob_MetacharactersInSearchPath(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "app/[slug]/page.tsx", "export default function Page() {}\n")

	tool := Glob(WithWorkDir(dir))
	result := call(t, tool, `{"pattern": "*.tsx", "path": "app/[slug]"}`)
	assertContains(t, result, "page.tsx")

	// Unbalanced bracket in the path is legal on disk and must not be
	// misreported as an invalid glob pattern.
	writeTestFile(t, dir, "ba[d/file.txt", "data\n")
	result = call(t, tool, `{"pattern": "*.txt", "path": "ba[d"}`)
	assertContains(t, result, "file.txt")

	// A genuinely malformed pattern still gets a retry error.
	if err := callErr(t, tool, `{"pattern": "[unclosed", "path": "app"}`); err == nil {
		t.Fatal("expected error for malformed pattern")
	}
}

// TestExpandBraces_MultipleGroups verifies sequential brace groups expand as
// a cartesian product instead of leaving later groups literal.
func TestExpandBraces_MultipleGroups(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"{a,b}/*.{go,mod}", []string{"a/*.go", "a/*.mod", "b/*.go", "b/*.mod"}},
		{"*.{a,b}.{c,d}", []string{"*.a.c", "*.a.d", "*.b.c", "*.b.d"}},
		{"{x,y}/{p,q}/z", []string{"x/p/z", "x/q/z", "y/p/z", "y/q/z"}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := expandBraces(tt.input)
			if len(got) != len(tt.expected) {
				t.Fatalf("expandBraces(%q) = %v, want %v", tt.input, got, tt.expected)
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Fatalf("expandBraces(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.expected[i])
				}
			}
		})
	}
}

// TestGlob_MultipleBraceGroups exercises a two-group pattern end to end.
func TestGlob_MultipleBraceGroups(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "cmd/x.go", "package x\n")
	writeTestFile(t, dir, "internal/y.mod", "module y\n")
	writeTestFile(t, dir, "internal/z.txt", "text\n")

	tool := Glob(WithWorkDir(dir))
	result := call(t, tool, `{"pattern": "{cmd,internal}/*.{go,mod}"}`)
	assertContains(t, result, "x.go")
	assertContains(t, result, "y.mod")
	assertNotContains(t, result, "z.txt")
}

// TestLs_TruncationSingleMarker verifies that when the 500-entry cap is
// crossed inside a nested directory, ls emits exactly one truncation marker
// and at most 500 entry lines (no per-recursion-level duplicates).
func TestLs_TruncationSingleMarker(t *testing.T) {
	dir := t.TempDir()
	// 497 top-level files sort before "sub", so the cap is crossed two
	// directory levels deep: 497 files + sub/ + sub/deep/ = 499 lines,
	// then deep's files push past 500.
	for i := range 497 {
		writeTestFile(t, dir, fmt.Sprintf("f%03d.txt", i), "x\n")
	}
	for i := range 20 {
		writeTestFile(t, dir, fmt.Sprintf("sub/deep/g%03d.txt", i), "x\n")
	}

	tool := Ls(WithWorkDir(dir))
	result := call(t, tool, `{"depth": 3}`)

	const marker = "... (truncated at 500 entries)"
	if n := strings.Count(result, marker); n != 1 {
		t.Fatalf("expected exactly 1 truncation marker, got %d:\n%s", n, result)
	}
	lines := strings.Split(result, "\n")
	if lines[len(lines)-1] != marker {
		t.Fatalf("expected output to end with the truncation marker, got %q", lines[len(lines)-1])
	}
	if entries := len(lines) - 1; entries != 500 {
		t.Fatalf("expected exactly 500 entry lines, got %d", entries)
	}
	// Truncated output must not also carry the "(N entries)" footer.
	if strings.Contains(result, "\n(") {
		t.Fatalf("unexpected entry-count footer in truncated output:\n%s", result)
	}
}

// TestLs_ExactlyAtCapNotTruncated verifies a tree with exactly 500 entries
// is reported in full with the entry-count footer and no truncation marker.
func TestLs_ExactlyAtCapNotTruncated(t *testing.T) {
	dir := t.TempDir()
	for i := range 500 {
		writeTestFile(t, dir, fmt.Sprintf("f%03d.txt", i), "x\n")
	}

	tool := Ls(WithWorkDir(dir))
	result := call(t, tool, `{}`)

	assertNotContains(t, result, "truncated at 500 entries")
	assertContains(t, result, "(500 entries)")
}
