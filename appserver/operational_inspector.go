package appserver

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/fugue-labs/gollem/appserver/protocol"
	toolgit "github.com/fugue-labs/gollem/appserver/tools/git"
	toolprocess "github.com/fugue-labs/gollem/appserver/tools/process"
)

const (
	operationalListDefaultLimit = 32
	operationalListMaxLimit     = 32
	operationalCursorMaxBytes   = 2048
	operationalCommandMaxBytes  = 256
	operationalTerminateDomain  = "gollem.background-terminal.terminate.v1\x00"
)

type operationalCursor struct {
	Version    int    `json:"version"`
	Kind       string `json:"kind"`
	SnapshotID string `json:"snapshotId"`
	Offset     int    `json:"offset"`
}

func decodeOperationalListParams(raw json.RawMessage) (protocol.OperationalListParams, *protocol.Error) {
	var params protocol.OperationalListParams
	if err := decodeOperationalParams(raw, &params); err != nil {
		return protocol.OperationalListParams{}, invalidParams("invalid operational list params", err)
	}
	if params.Limit < 0 || params.Limit > operationalListMaxLimit {
		return protocol.OperationalListParams{}, invalidParams(
			fmt.Sprintf("limit must be between 1 and %d when provided", operationalListMaxLimit), nil,
		)
	}
	if len(params.Cursor) > operationalCursorMaxBytes {
		return protocol.OperationalListParams{}, invalidParams(
			fmt.Sprintf("cursor exceeds %d bytes", operationalCursorMaxBytes), nil,
		)
	}
	return params, nil
}

func decodeOperationalParams(raw json.RawMessage, out any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}

func rejectOperationalParams(raw json.RawMessage) *protocol.Error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var value map[string]json.RawMessage
	if err := decodeOperationalParams(raw, &value); err != nil {
		return invalidParams("invalid operational params", err)
	}
	if len(value) != 0 {
		return invalidParams("method does not accept params", nil)
	}
	return nil
}

func operationalPageBounds(
	params protocol.OperationalListParams,
	kind string,
	snapshotID string,
	total int,
) (int, int, string, *protocol.Error) {
	limit := params.Limit
	if limit == 0 {
		limit = operationalListDefaultLimit
	}
	offset := 0
	if params.Cursor != "" {
		cursor, err := decodeOperationalCursor(params.Cursor)
		if err != nil {
			return 0, 0, "", invalidParams("invalid operational cursor", err)
		}
		if cursor.Kind != kind {
			return 0, 0, "", invalidParams("operational cursor belongs to another method", nil)
		}
		if cursor.SnapshotID != snapshotID {
			return 0, 0, "", invalidParams("operational cursor snapshot is stale", nil)
		}
		if cursor.Offset < 0 || cursor.Offset > total {
			return 0, 0, "", invalidParams("operational cursor offset is outside the snapshot", nil)
		}
		offset = cursor.Offset
	}
	end := offset + limit
	if end > total {
		end = total
	}
	nextCursor := ""
	if end < total {
		nextCursor = encodeOperationalCursor(operationalCursor{
			Version:    1,
			Kind:       kind,
			SnapshotID: snapshotID,
			Offset:     end,
		})
	}
	return offset, end, nextCursor, nil
}

func decodeOperationalCursor(value string) (operationalCursor, error) {
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return operationalCursor{}, fmt.Errorf("decode cursor: %w", err)
	}
	var cursor operationalCursor
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cursor); err != nil {
		return operationalCursor{}, fmt.Errorf("decode cursor payload: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return operationalCursor{}, errors.New("decode cursor payload: trailing data")
		}
		return operationalCursor{}, fmt.Errorf("decode cursor payload: %w", err)
	}
	if cursor.Version != 1 || cursor.Kind == "" || cursor.SnapshotID == "" {
		return operationalCursor{}, errors.New("decode cursor payload: unsupported shape")
	}
	return cursor, nil
}

func encodeOperationalCursor(cursor operationalCursor) string {
	data, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(data)
}

func operationalSnapshotID(kind string, value any) string {
	data, _ := json.Marshal(struct {
		Kind  string `json:"kind"`
		Value any    `json:"value"`
	}{Kind: kind, Value: value})
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func operationalTerminalTerminateApprovalItemID(id string) string {
	sum := sha256.Sum256([]byte(operationalTerminateDomain + id))
	return "terminal-terminate-sha256:" + hex.EncodeToString(sum[:])
}

func operationalBackgroundTerminals(root string, snapshots []toolprocess.Snapshot) []protocol.BackgroundTerminal {
	terminals := make([]protocol.BackgroundTerminal, 0, len(snapshots))
	for i := range snapshots {
		terminals = append(terminals, operationalBackgroundTerminal(root, &snapshots[i]))
	}
	return terminals
}

func operationalBackgroundTerminal(root string, snapshot *toolprocess.Snapshot) protocol.BackgroundTerminal {
	if snapshot == nil {
		return protocol.BackgroundTerminal{}
	}
	command, commandRedacted, commandTruncated := operationalCommandLabel(snapshot)
	workDir, workDirRedacted, workDirTruncated := operationalWorkDir(root, snapshot.WorkDir)
	result := protocol.BackgroundTerminal{
		ID:                snapshot.ID,
		TerminalID:        snapshot.ID,
		ProcessID:         snapshot.ID,
		PID:               snapshot.PID,
		Title:             command,
		Command:           command,
		WorkDir:           workDir,
		Status:            protocol.BackgroundTerminalStatus(snapshot.Status),
		StartedAt:         snapshot.StartedAt,
		ArgumentCount:     len(snapshot.Args),
		CommandRedacted:   commandRedacted,
		MetadataTruncated: commandTruncated || workDirTruncated || workDirRedacted,
	}
	if snapshot.Status != toolprocess.StatusRunning {
		exitCode := snapshot.ExitCode
		result.ExitCode = &exitCode
	}
	if !snapshot.EndedAt.IsZero() {
		endedAt := snapshot.EndedAt
		result.EndedAt = &endedAt
	}
	return result
}

func operationalCommandLabel(snapshot *toolprocess.Snapshot) (string, bool, bool) {
	if snapshot == nil {
		return "process", true, false
	}
	raw := strings.TrimSpace(snapshot.Command)
	if snapshot.Shell {
		return "shell command", true, false
	}
	label := filepath.Base(raw)
	if label == "." || label == string(filepath.Separator) || label == "" {
		label = "process"
	}
	label, normalized := operationalDisplayText(label)
	bounded, truncated := boundedRuntimeProcessMetadata(label, operationalCommandMaxBytes)
	return bounded, bounded != raw || len(snapshot.Args) > 0 || normalized, truncated
}

func operationalWorkDir(root, workDir string) (string, bool, bool) {
	root = strings.TrimSpace(root)
	workDir = strings.TrimSpace(workDir)
	if workDir == "" {
		return ".", false, false
	}
	value := workDir
	redacted := false
	if filepath.IsAbs(value) {
		relative, err := filepath.Rel(root, value)
		if root == "" || err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			value = "."
			redacted = true
		} else {
			value = relative
			if value == "" {
				value = "."
			}
			redacted = true
		}
	} else {
		value = filepath.Clean(value)
		if value == ".." || strings.HasPrefix(value, ".."+string(filepath.Separator)) {
			value = "."
			redacted = true
		}
	}
	value, normalized := operationalDisplayText(value)
	redacted = redacted || normalized
	bounded, truncated := boundedRuntimeProcessMetadata(value, runtimeProcessMetadataMaxBytes)
	return bounded, redacted, truncated
}

func cloneOperationalTerminals(in []protocol.BackgroundTerminal) []protocol.BackgroundTerminal {
	out := make([]protocol.BackgroundTerminal, len(in))
	copy(out, in)
	return out
}

func boundedOperationalTerminals(in []protocol.BackgroundTerminal) ([]protocol.BackgroundTerminal, bool) {
	if len(in) > operationalListMaxLimit {
		return cloneOperationalTerminals(in[:operationalListMaxLimit]), true
	}
	return cloneOperationalTerminals(in), false
}

func operationalGitStatus(status *toolgit.Status) (string, bool, []protocol.GitStatusEntry, bool) {
	if status == nil {
		return "", false, []protocol.GitStatusEntry{}, true
	}
	branchValue, branchNormalized := operationalDisplayText(status.BranchLine)
	branch, branchTruncated := boundedRuntimeProcessMetadata(branchValue, runtimeGitMetadataMaxBytes)
	entries := make([]protocol.GitStatusEntry, 0, len(status.Entries))
	for _, entry := range status.Entries {
		codeValue, codeNormalized := operationalDisplayText(entry.Code)
		pathValue, pathNormalized := operationalDisplayText(entry.Path)
		code, codeTruncated := boundedRuntimeProcessMetadata(codeValue, 8)
		path, pathTruncated := boundedRuntimeProcessMetadata(pathValue, runtimeGitMetadataMaxBytes)
		entries = append(entries, protocol.GitStatusEntry{
			Code:      code,
			Path:      path,
			Truncated: codeTruncated || pathTruncated || codeNormalized || pathNormalized,
		})
	}
	return branch, branchTruncated || branchNormalized, entries, status.Clean
}

func operationalGitWorktrees(worktrees []toolgit.Worktree) []protocol.GitWorktree {
	result := make([]protocol.GitWorktree, 0, len(worktrees))
	for i := range worktrees {
		pathValue, pathNormalized := operationalDisplayText(worktrees[i].Path)
		headValue, headNormalized := operationalDisplayText(worktrees[i].Head)
		branchValue, branchNormalized := operationalDisplayText(worktrees[i].Branch)
		path, pathTruncated := boundedRuntimeProcessMetadata(pathValue, runtimeGitMetadataMaxBytes)
		head, headTruncated := boundedRuntimeProcessMetadata(headValue, runtimeGitMetadataMaxBytes)
		branch, branchTruncated := boundedRuntimeProcessMetadata(branchValue, runtimeGitMetadataMaxBytes)
		result = append(result, protocol.GitWorktree{
			Path:     path,
			Head:     head,
			Branch:   branch,
			Detached: worktrees[i].Detached,
			Bare:     worktrees[i].Bare,
			MetadataTruncated: pathTruncated || headTruncated || branchTruncated ||
				pathNormalized || headNormalized || branchNormalized,
		})
	}
	return result
}

func operationalDisplayText(value string) (string, bool) {
	valid := strings.ToValidUTF8(value, "\uFFFD")
	normalized := valid != value
	result := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			normalized = true
			return '\uFFFD'
		}
		return r
	}, valid)
	return result, normalized
}

func operationalObservedAt() time.Time {
	return time.Now().UTC()
}
