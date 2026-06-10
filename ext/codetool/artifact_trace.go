package codetool

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/fugue-labs/gollem/core"
)

const artifactTraceDiffMaxBytes = 64 * 1024

type artifactChangeEvidence struct {
	BeforeSHA256         string
	AfterSHA256          string
	Diff                 string
	DiffTruncated        bool
	DiffOmittedReason    string
	BeforeContent        string
	AfterContent         string
	ContentEncoding      string
	ContentTruncated     bool
	ContentOmittedReason string
}

func writeFileAndTrace(ctx context.Context, rc *core.RunContext, cfg *Config, path string, content []byte, perm os.FileMode, operation, toolName string) error {
	before, beforeExists, evidence := readArtifactBefore(path)
	evidence.AfterSHA256 = sha256Hex(content)
	diff, truncated, omitted := buildArtifactDiff(path, before, content, beforeExists)
	evidence.Diff = diff
	evidence.DiffTruncated = truncated
	if evidence.DiffOmittedReason == "" {
		evidence.DiffOmittedReason = omitted
	}
	beforeContent, afterContent, contentTruncated, contentOmitted := buildArtifactContentSnapshot(before, content, beforeExists)
	evidence.BeforeContent = beforeContent
	evidence.AfterContent = afterContent
	evidence.ContentEncoding = "utf-8"
	evidence.ContentTruncated = contentTruncated
	if contentOmitted != "" {
		evidence.ContentOmittedReason = contentOmitted
		evidence.ContentEncoding = ""
	}

	if err := os.WriteFile(path, content, perm); err != nil {
		return err
	}
	publishArtifactChanged(ctx, rc, cfg, path, operation, toolName, int64(len(content)), evidence)
	return nil
}

func publishArtifactChanged(ctx context.Context, rc *core.RunContext, cfg *Config, path, operation, toolName string, bytes int64, evidence artifactChangeEvidence) {
	bus := (*core.EventBus)(nil)
	if cfg != nil {
		bus = cfg.EventBus
	}
	if bus == nil && rc != nil {
		bus = rc.EventBus
	}
	if bus == nil {
		return
	}

	runID := core.RunIDFromContext(ctx)
	toolCallID := core.ToolCallIDFromContext(ctx)
	if rc != nil {
		if runID == "" {
			runID = rc.RunID
		}
		if toolCallID == "" {
			toolCallID = rc.ToolCallID
		}
		if toolName == "" {
			toolName = rc.ToolName
		}
	}

	var parentRunID string
	if rc != nil {
		parentRunID = rc.ParentRunID
	}

	core.Publish(bus, core.ArtifactChangedEvent{
		RunID:                runID,
		ParentRunID:          parentRunID,
		ToolCallID:           toolCallID,
		ToolName:             toolName,
		Path:                 path,
		Operation:            operation,
		Bytes:                bytes,
		BeforeSHA256:         evidence.BeforeSHA256,
		AfterSHA256:          evidence.AfterSHA256,
		Diff:                 evidence.Diff,
		DiffTruncated:        evidence.DiffTruncated,
		DiffOmittedReason:    evidence.DiffOmittedReason,
		BeforeContent:        evidence.BeforeContent,
		AfterContent:         evidence.AfterContent,
		ContentEncoding:      evidence.ContentEncoding,
		ContentTruncated:     evidence.ContentTruncated,
		ContentOmittedReason: evidence.ContentOmittedReason,
		ChangedAt:            time.Now(),
	})
}

func readArtifactBefore(path string) ([]byte, bool, artifactChangeEvidence) {
	evidence := artifactChangeEvidence{}
	info, err := os.Stat(path)
	if err != nil {
		if !os.IsNotExist(err) {
			evidence.DiffOmittedReason = "stat before: " + err.Error()
		}
		return nil, false, evidence
	}
	if info.IsDir() {
		evidence.DiffOmittedReason = "path is a directory"
		return nil, true, evidence
	}
	if hash, err := sha256File(path); err == nil {
		evidence.BeforeSHA256 = hash
	}
	if info.Size() > artifactTraceDiffMaxBytes {
		evidence.DiffOmittedReason = fmt.Sprintf("before file exceeds %d byte trace diff limit", artifactTraceDiffMaxBytes)
		return nil, true, evidence
	}
	before, err := os.ReadFile(path)
	if err != nil {
		evidence.DiffOmittedReason = "read before: " + err.Error()
		return nil, true, evidence
	}
	return before, true, evidence
}

func buildArtifactDiff(path string, before, after []byte, beforeExists bool) (string, bool, string) {
	if beforeExists && bytes.Equal(before, after) {
		return "", false, ""
	}
	if len(after) > artifactTraceDiffMaxBytes {
		return "", false, fmt.Sprintf("after file exceeds %d byte trace diff limit", artifactTraceDiffMaxBytes)
	}
	if beforeExists && before == nil {
		return "", false, "before content unavailable"
	}
	if !traceDiffText(before) || !traceDiffText(after) {
		return "", false, "binary content omitted"
	}

	oldPath := "/dev/null"
	if beforeExists {
		oldPath = "a/" + traceDiffPath(path)
	}
	newPath := "b/" + traceDiffPath(path)
	beforeLines := splitDiffLines(string(before))
	afterLines := splitDiffLines(string(after))

	var b strings.Builder
	fmt.Fprintf(&b, "--- %s\n", oldPath)
	fmt.Fprintf(&b, "+++ %s\n", newPath)
	writeUnifiedTraceHunk(&b, beforeLines, afterLines, beforeExists)
	diff := b.String()
	if len(diff) <= artifactTraceDiffMaxBytes {
		return diff, false, ""
	}
	return diff[:artifactTraceDiffMaxBytes] + "\n... diff truncated ...\n", true, ""
}

func buildArtifactContentSnapshot(before, after []byte, beforeExists bool) (string, string, bool, string) {
	if beforeExists && before == nil {
		return "", "", false, "before content unavailable"
	}
	if !traceDiffText(before) || !traceDiffText(after) {
		return "", "", false, "binary content omitted"
	}
	beforeContent, beforeTruncated := boundedArtifactContent(before)
	afterContent, afterTruncated := boundedArtifactContent(after)
	return beforeContent, afterContent, beforeTruncated || afterTruncated, ""
}

func boundedArtifactContent(data []byte) (string, bool) {
	if len(data) <= artifactTraceDiffMaxBytes {
		return string(data), false
	}
	return string(data[:artifactTraceDiffMaxBytes]), true
}

func splitDiffLines(content string) []string {
	if content == "" {
		return nil
	}
	lines := strings.SplitAfter(content, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func writeUnifiedTraceHunk(b *strings.Builder, beforeLines, afterLines []string, beforeExists bool) {
	if !beforeExists {
		fmt.Fprintf(b, "@@ -0,0 +1,%d @@\n", len(afterLines))
		for _, line := range afterLines {
			writeDiffLine(b, '+', line)
		}
		return
	}

	prefix := commonPrefixLines(beforeLines, afterLines)
	suffix := commonSuffixLines(beforeLines, afterLines, prefix)
	const contextLines = 3
	oldChangeEnd := len(beforeLines) - suffix
	newChangeEnd := len(afterLines) - suffix
	oldStart := max(0, prefix-contextLines)
	newStart := max(0, prefix-contextLines)
	oldEnd := min(len(beforeLines), oldChangeEnd+contextLines)
	newEnd := min(len(afterLines), newChangeEnd+contextLines)

	fmt.Fprintf(
		b,
		"@@ -%d,%d +%d,%d @@\n",
		diffRangeStart(oldStart, oldEnd),
		oldEnd-oldStart,
		diffRangeStart(newStart, newEnd),
		newEnd-newStart,
	)
	for i := oldStart; i < prefix; i++ {
		writeDiffLine(b, ' ', beforeLines[i])
	}
	for i := prefix; i < oldChangeEnd; i++ {
		writeDiffLine(b, '-', beforeLines[i])
	}
	for i := prefix; i < newChangeEnd; i++ {
		writeDiffLine(b, '+', afterLines[i])
	}
	for i := oldChangeEnd; i < oldEnd; i++ {
		writeDiffLine(b, ' ', beforeLines[i])
	}
}

func commonPrefixLines(left, right []string) int {
	limit := min(len(left), len(right))
	for i := range limit {
		if left[i] != right[i] {
			return i
		}
	}
	return limit
}

func commonSuffixLines(left, right []string, prefix int) int {
	limit := min(len(left), len(right)) - prefix
	for i := range limit {
		if left[len(left)-1-i] != right[len(right)-1-i] {
			return i
		}
	}
	return limit
}

func diffRangeStart(start, end int) int {
	if end == 0 {
		return 0
	}
	return start + 1
}

func writeDiffLine(b *strings.Builder, prefix byte, line string) {
	b.WriteByte(prefix)
	b.WriteString(line)
	if !strings.HasSuffix(line, "\n") {
		b.WriteByte('\n')
	}
}

func traceDiffPath(path string) string {
	path = filepath.ToSlash(filepath.Clean(path))
	path = strings.TrimPrefix(path, "/")
	if path == "." || path == "" {
		return "artifact"
	}
	return path
}

func traceDiffText(data []byte) bool {
	return !bytes.Contains(data, []byte{0}) && utf8.Valid(data)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
