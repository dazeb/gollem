package fs

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	iofs "io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	ErrPathOutsideRoot        = errors.New("appserver/fs: path escapes workspace root")
	ErrApprovalDenied         = errors.New("appserver/fs: operation denied by approval policy")
	ErrRefusingRoot           = errors.New("appserver/fs: refusing to mutate workspace root")
	ErrInvalidCopyDestination = errors.New("appserver/fs: invalid copy destination")
	ErrRecursiveRequired      = errors.New("appserver/fs: recursive option is required for directory copy")
	ErrWatchPathNotAbsolute   = errors.New("appserver/fs: watch path must be absolute")
	ErrWatchIDRequired        = errors.New("appserver/fs: watch id is required")
	ErrWatchAlreadyExists     = errors.New("appserver/fs: watch id already exists")
	ErrWatchNotFound          = errors.New("appserver/fs: watch id not found")
	ErrInvalidMutationScope   = errors.New("appserver/fs: invalid approved mutation scope")
	ErrExactStateMismatch     = errors.New("appserver/fs: current file does not match expected state")
	ErrExactRevertSymlink     = errors.New("appserver/fs: exact revert refuses symlinks")
	ErrExactRevertUnsupported = errors.New("appserver/fs: exact revert supports regular files only")
	ErrExactRevertPending     = errors.New("appserver/fs: exact revert has pending recovery state")
)

const exactFileModeMask = iofs.ModePerm | iofs.ModeSetuid | iofs.ModeSetgid | iofs.ModeSticky

type OperationKind string

const (
	OperationReadFile         OperationKind = "readFile"
	OperationWriteFile        OperationKind = "writeFile"
	OperationCreateDirectory  OperationKind = "createDirectory"
	OperationReadDirectory    OperationKind = "readDirectory"
	OperationMetadata         OperationKind = "getMetadata"
	OperationRemove           OperationKind = "remove"
	OperationCopy             OperationKind = "copy"
	OperationWatch            OperationKind = "watch"
	OperationUnwatch          OperationKind = "unwatch"
	OperationRevertFileChange OperationKind = "revertFileChange"
)

type Operation struct {
	Kind        OperationKind
	Path        string
	Destination string
	Destructive bool
}

type AuditEvent struct {
	Operation   Operation
	Resolved    string
	Destination string
	Allowed     bool
	Err         string
	At          time.Time
}

type ApprovalFunc func(context.Context, Operation) error
type AuditSink func(AuditEvent)

type Option func(*Service)

func WithApproval(fn ApprovalFunc) Option {
	return func(s *Service) {
		s.approve = fn
	}
}

func WithAuditSink(fn AuditSink) Option {
	return func(s *Service) {
		s.audit = fn
	}
}

type Service struct {
	root     string
	rootEval string
	approve  ApprovalFunc
	audit    AuditSink

	mu      sync.Mutex
	watches map[string]*watchRegistration

	mutationMu sync.Mutex
}

type approvedMutationScopeKey struct{}

type approvedMutationScope struct {
	mu        sync.Mutex
	service   *Service
	operation Operation
	active    bool
	used      bool
}

type FileContent struct {
	Path    string
	Content []byte
	Size    int64
	Mode    iofs.FileMode
	ModTime time.Time
}

type DirEntry struct {
	Path    string
	Name    string
	IsDir   bool
	IsFile  bool
	Size    int64
	Mode    iofs.FileMode
	ModTime time.Time
}

type Metadata struct {
	Path      string
	IsDir     bool
	IsFile    bool
	IsSymlink bool
	Size      int64
	Mode      iofs.FileMode
	ModTime   time.Time
}

type CreateDirectoryOptions struct {
	Recursive bool
}

type RemoveOptions struct {
	Recursive bool
	Force     bool
}

type CopyOptions struct {
	Recursive bool
}

type ExactFileState struct {
	Exists    bool
	SHA256    string
	Content   []byte
	Mode      iofs.FileMode
	CheckMode bool
}

type RevertFileRequest struct {
	Path          string
	TransactionID string
	Before        ExactFileState
	After         ExactFileState
}

type RevertFileResult struct {
	Path     string
	SHA256   string
	Restored bool
	Removed  bool
}

func NewService(root string, opts ...Option) (*Service, error) {
	if root == "" {
		return nil, errors.New("appserver/fs: root must not be empty")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve root: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("create root: %w", err)
	}
	eval, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("evaluate root: %w", err)
	}
	s := &Service{root: abs, rootEval: eval}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

func (s *Service) Root() string {
	return s.root
}

// RunApprovedMutation waits for approval before entering the mutation lock,
// then lets callers capture before/after evidence around exactly one matching
// service mutation. The scoped context cannot bypass approval after fn returns.
func (s *Service) RunApprovedMutation(
	ctx context.Context,
	op Operation,
	fn func(context.Context) error,
) error {
	if s == nil {
		return errors.New("appserver/fs: nil service")
	}
	if fn == nil {
		return errors.New("appserver/fs: mutation callback is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	resolved, destination, err := s.resolveMutationOperation(op)
	if err != nil {
		s.emit(op, resolved, destination, false, err)
		return err
	}
	if err := s.requireApproval(ctx, op); err != nil {
		s.emit(op, resolved, destination, false, err)
		return err
	}
	if err := checkContext(ctx); err != nil {
		s.emit(op, resolved, destination, false, err)
		return err
	}

	s.mutationMu.Lock()
	scope := &approvedMutationScope{
		service:   s,
		operation: op,
		active:    true,
	}
	scopedCtx := context.WithValue(ctx, approvedMutationScopeKey{}, scope)
	defer func() {
		scope.mu.Lock()
		scope.active = false
		scope.mu.Unlock()
		s.mutationMu.Unlock()
	}()
	return fn(scopedCtx)
}

func (s *Service) ReadFile(ctx context.Context, path string) (*FileContent, error) {
	op := Operation{Kind: OperationReadFile, Path: path}
	if err := checkContext(ctx); err != nil {
		s.emit(op, "", "", false, err)
		return nil, err
	}
	resolved, err := s.resolve(path)
	if err != nil {
		s.emit(op, "", "", false, err)
		return nil, err
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		s.emit(op, resolved, "", false, err)
		return nil, fmt.Errorf("read file: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		s.emit(op, resolved, "", false, err)
		return nil, fmt.Errorf("stat file: %w", err)
	}
	s.emit(op, resolved, "", true, nil)
	return &FileContent{
		Path:    s.rel(resolved),
		Content: data,
		Size:    info.Size(),
		Mode:    info.Mode(),
		ModTime: info.ModTime(),
	}, nil
}

func (s *Service) WriteFile(ctx context.Context, path string, content []byte, perm iofs.FileMode) error {
	if perm == 0 {
		perm = 0o644
	}
	op := Operation{Kind: OperationWriteFile, Path: path}
	release, approved, err := s.enterMutation(ctx, op)
	if err != nil {
		s.emit(op, "", "", false, err)
		return err
	}
	defer release()
	if err := checkContext(ctx); err != nil {
		s.emit(op, "", "", false, err)
		return err
	}
	resolved, err := s.resolve(path)
	if err != nil {
		s.emit(op, "", "", false, err)
		return err
	}
	if !approved {
		err = s.requireApproval(ctx, op)
	}
	if err != nil {
		s.emit(op, resolved, "", false, err)
		return err
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		s.emit(op, resolved, "", false, err)
		return fmt.Errorf("create parent directory: %w", err)
	}
	if err := os.WriteFile(resolved, content, perm); err != nil {
		s.emit(op, resolved, "", false, err)
		return fmt.Errorf("write file: %w", err)
	}
	s.emit(op, resolved, "", true, nil)
	return nil
}

func (s *Service) CreateDirectory(ctx context.Context, path string) error {
	return s.CreateDirectoryWithOptions(ctx, path, CreateDirectoryOptions{Recursive: true})
}

func (s *Service) CreateDirectoryWithOptions(ctx context.Context, path string, options CreateDirectoryOptions) error {
	op := Operation{Kind: OperationCreateDirectory, Path: path}
	release, approved, err := s.enterMutation(ctx, op)
	if err != nil {
		s.emit(op, "", "", false, err)
		return err
	}
	defer release()
	if err := checkContext(ctx); err != nil {
		s.emit(op, "", "", false, err)
		return err
	}
	resolved, err := s.resolve(path)
	if err != nil {
		s.emit(op, "", "", false, err)
		return err
	}
	if !approved {
		err = s.requireApproval(ctx, op)
	}
	if err != nil {
		s.emit(op, resolved, "", false, err)
		return err
	}
	create := os.Mkdir
	if options.Recursive {
		create = os.MkdirAll
	}
	if err := create(resolved, 0o755); err != nil {
		s.emit(op, resolved, "", false, err)
		return fmt.Errorf("create directory: %w", err)
	}
	s.emit(op, resolved, "", true, nil)
	return nil
}

func (s *Service) ReadDirectory(ctx context.Context, path string) ([]DirEntry, error) {
	op := Operation{Kind: OperationReadDirectory, Path: path}
	if err := checkContext(ctx); err != nil {
		s.emit(op, "", "", false, err)
		return nil, err
	}
	resolved, err := s.resolve(path)
	if err != nil {
		s.emit(op, "", "", false, err)
		return nil, err
	}
	entries, err := os.ReadDir(resolved)
	if err != nil {
		s.emit(op, resolved, "", false, err)
		return nil, fmt.Errorf("read directory: %w", err)
	}
	out := make([]DirEntry, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			s.emit(op, resolved, "", false, err)
			return nil, fmt.Errorf("read directory entry: %w", err)
		}
		child := filepath.Join(resolved, entry.Name())
		out = append(out, DirEntry{
			Path:    s.rel(child),
			Name:    entry.Name(),
			IsDir:   entry.IsDir(),
			IsFile:  info.Mode().IsRegular(),
			Size:    info.Size(),
			Mode:    info.Mode(),
			ModTime: info.ModTime(),
		})
	}
	s.emit(op, resolved, "", true, nil)
	return out, nil
}

func (s *Service) Metadata(ctx context.Context, path string) (*Metadata, error) {
	op := Operation{Kind: OperationMetadata, Path: path}
	if err := checkContext(ctx); err != nil {
		s.emit(op, "", "", false, err)
		return nil, err
	}
	resolved, err := s.resolve(path)
	if err != nil {
		s.emit(op, "", "", false, err)
		return nil, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		s.emit(op, resolved, "", false, err)
		return nil, fmt.Errorf("stat path: %w", err)
	}
	s.emit(op, resolved, "", true, nil)
	return &Metadata{
		Path:      s.rel(resolved),
		IsDir:     info.IsDir(),
		IsFile:    info.Mode().IsRegular(),
		IsSymlink: isSymlink(resolved),
		Size:      info.Size(),
		Mode:      info.Mode(),
		ModTime:   info.ModTime(),
	}, nil
}

func (s *Service) Remove(ctx context.Context, path string) error {
	return s.RemoveWithOptions(ctx, path, RemoveOptions{Recursive: true, Force: true})
}

func (s *Service) RemoveWithOptions(ctx context.Context, path string, options RemoveOptions) error {
	op := Operation{Kind: OperationRemove, Path: path, Destructive: true}
	release, approved, err := s.enterMutation(ctx, op)
	if err != nil {
		s.emit(op, "", "", false, err)
		return err
	}
	defer release()
	if err := checkContext(ctx); err != nil {
		s.emit(op, "", "", false, err)
		return err
	}
	resolved, err := s.resolve(path)
	if err != nil {
		s.emit(op, "", "", false, err)
		return err
	}
	if samePath(resolved, s.root) {
		s.emit(op, resolved, "", false, ErrRefusingRoot)
		return ErrRefusingRoot
	}
	if !approved {
		err = s.requireApproval(ctx, op)
	}
	if err != nil {
		s.emit(op, resolved, "", false, err)
		return err
	}
	var removeErr error
	if options.Recursive {
		if !options.Force {
			if _, err := os.Lstat(resolved); err != nil {
				removeErr = err
			}
		}
		if removeErr == nil {
			removeErr = os.RemoveAll(resolved)
		}
	} else {
		removeErr = os.Remove(resolved)
		if options.Force && errors.Is(removeErr, os.ErrNotExist) {
			removeErr = nil
		}
	}
	if removeErr != nil {
		s.emit(op, resolved, "", false, removeErr)
		return fmt.Errorf("remove path: %w", removeErr)
	}
	s.emit(op, resolved, "", true, nil)
	return nil
}

func (s *Service) Copy(ctx context.Context, src, dst string) error {
	return s.CopyWithOptions(ctx, src, dst, CopyOptions{Recursive: true})
}

func (s *Service) CopyWithOptions(ctx context.Context, src, dst string, options CopyOptions) error {
	op := Operation{Kind: OperationCopy, Path: src, Destination: dst}
	release, approved, err := s.enterMutation(ctx, op)
	if err != nil {
		s.emit(op, "", "", false, err)
		return err
	}
	defer release()
	if err := checkContext(ctx); err != nil {
		s.emit(op, "", "", false, err)
		return err
	}
	resolvedSrc, err := s.resolve(src)
	if err != nil {
		s.emit(op, "", "", false, err)
		return err
	}
	resolvedDst, err := s.resolve(dst)
	if err != nil {
		s.emit(op, resolvedSrc, "", false, err)
		return err
	}
	info, err := os.Stat(resolvedSrc)
	if err != nil {
		s.emit(op, resolvedSrc, resolvedDst, false, err)
		return fmt.Errorf("stat source: %w", err)
	}
	if samePath(resolvedSrc, resolvedDst) || (info.IsDir() && pathInside(resolvedSrc, resolvedDst)) {
		s.emit(op, resolvedSrc, resolvedDst, false, ErrInvalidCopyDestination)
		return ErrInvalidCopyDestination
	}
	if info.IsDir() && !options.Recursive {
		s.emit(op, resolvedSrc, resolvedDst, false, ErrRecursiveRequired)
		return ErrRecursiveRequired
	}
	if !approved {
		err = s.requireApproval(ctx, op)
	}
	if err != nil {
		s.emit(op, resolvedSrc, resolvedDst, false, err)
		return err
	}
	if info.IsDir() {
		err = copyDir(resolvedSrc, resolvedDst)
	} else {
		err = copyFile(resolvedSrc, resolvedDst, info.Mode())
	}
	if err != nil {
		s.emit(op, resolvedSrc, resolvedDst, false, err)
		return err
	}
	s.emit(op, resolvedSrc, resolvedDst, true, nil)
	return nil
}

// RevertFile restores one exact regular-file state after verifying that the
// current file still matches the expected post-change digest. It refuses all
// symlinks and never removes directories.
func (s *Service) RevertFile(ctx context.Context, req RevertFileRequest) (*RevertFileResult, error) {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()

	op := Operation{Kind: OperationRevertFileChange, Path: req.Path, Destructive: true}
	fail := func(resolved string, err error) (*RevertFileResult, error) {
		s.emit(op, resolved, "", false, err)
		return nil, err
	}
	if err := checkContext(ctx); err != nil {
		return fail("", err)
	}
	if strings.TrimSpace(req.Path) == "" {
		return fail("", errors.New("appserver/fs: revert path is required"))
	}
	if strings.TrimSpace(req.TransactionID) == "" {
		return fail("", errors.New("appserver/fs: revert transaction id is required"))
	}
	if req.Before.Exists {
		if req.Before.SHA256 == "" || exactSHA256(req.Before.Content) != req.Before.SHA256 {
			return fail("", errors.New("appserver/fs: before-state digest is inconsistent"))
		}
	} else if len(req.Before.Content) != 0 || req.Before.SHA256 != "" {
		return fail("", errors.New("appserver/fs: absent before state cannot include content or digest"))
	}
	if req.After.Exists && req.After.SHA256 == "" {
		return fail("", errors.New("appserver/fs: expected after-state digest is required"))
	}
	resolved, err := s.resolve(req.Path)
	if err != nil {
		return fail("", err)
	}
	if samePath(resolved, s.root) {
		return fail(resolved, ErrRefusingRoot)
	}
	if err := rejectSymlinkComponents(s.root, resolved); err != nil {
		return fail(resolved, err)
	}
	if err := verifyExactFileState(resolved, req.After); err != nil {
		return fail(resolved, err)
	}
	if err := s.requireApproval(ctx, op); err != nil {
		return fail(resolved, err)
	}
	if err := checkContext(ctx); err != nil {
		return fail(resolved, err)
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return fail(resolved, fmt.Errorf("open exact revert root: %w", err))
	}
	defer root.Close()
	relative := filepath.Clean(s.rel(resolved))
	if filepath.IsAbs(relative) || relative == "." || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fail(resolved, ErrPathOutsideRoot)
	}
	if err := rejectRootSymlinkComponents(root, relative); err != nil {
		return fail(resolved, err)
	}
	if err := verifyExactRootFileState(root, relative, req.After); err != nil {
		return fail(resolved, err)
	}
	result := &RevertFileResult{Path: s.rel(resolved)}
	if req.Before.Exists {
		parent := filepath.Dir(relative)
		info, err := root.Lstat(parent)
		if err != nil {
			return fail(resolved, fmt.Errorf("stat revert parent: %w", err))
		}
		if !info.IsDir() || info.Mode()&iofs.ModeSymlink != 0 {
			return fail(resolved, ErrExactRevertUnsupported)
		}
		if err := atomicWriteRootFile(root, relative, req.Before.Content, req.Before.Mode, req.After, req.TransactionID); err != nil {
			return fail(resolved, err)
		}
		result.Restored = true
		result.SHA256 = req.Before.SHA256
	} else {
		if err := removeExactRootFile(root, relative, req.After, req.TransactionID); err != nil {
			return fail(resolved, err)
		}
		result.Removed = true
	}
	if err := verifyExactRootFileState(root, relative, req.Before); err != nil {
		return fail(resolved, fmt.Errorf("verify reverted file: %w", err))
	}
	s.emit(op, resolved, "", true, nil)
	return result, nil
}

// RecoverPendingRevert repairs or cleans a deterministic quarantine left by an
// interrupted exact revert. It never overwrites an unexpected target state.
func (s *Service) RecoverPendingRevert(ctx context.Context, req RevertFileRequest) error {
	if s == nil {
		return errors.New("appserver/fs: nil service")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := checkContext(ctx); err != nil {
		return err
	}
	if strings.TrimSpace(req.Path) == "" {
		return errors.New("appserver/fs: revert path is required")
	}
	if strings.TrimSpace(req.TransactionID) == "" {
		return errors.New("appserver/fs: revert transaction id is required")
	}
	if req.Before.Exists {
		if req.Before.SHA256 == "" || exactSHA256(req.Before.Content) != req.Before.SHA256 {
			return errors.New("appserver/fs: before-state digest is inconsistent")
		}
	} else if len(req.Before.Content) != 0 || req.Before.SHA256 != "" {
		return errors.New("appserver/fs: absent before state cannot include content or digest")
	}
	if req.After.Exists && req.After.SHA256 == "" {
		return errors.New("appserver/fs: expected after-state digest is required")
	}

	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()

	resolved, err := s.resolve(req.Path)
	if err != nil {
		return err
	}
	if samePath(resolved, s.root) {
		return ErrRefusingRoot
	}
	if err := rejectSymlinkComponents(s.root, resolved); err != nil {
		return err
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return fmt.Errorf("open exact revert recovery root: %w", err)
	}
	defer root.Close()
	relative := filepath.Clean(s.rel(resolved))
	if filepath.IsAbs(relative) || relative == "." || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return ErrPathOutsideRoot
	}
	if err := rejectRootSymlinkComponents(root, relative); err != nil {
		return err
	}

	transactionDir, quarantine, replacement := exactRevertTransactionPaths(relative, req.TransactionID)
	info, err := root.Lstat(transactionDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat exact revert transaction: %w", err)
	}
	if !info.IsDir() || info.Mode()&iofs.ModeSymlink != 0 {
		return ErrExactRevertPending
	}

	if _, err := root.Lstat(quarantine); errors.Is(err, os.ErrNotExist) {
		if err := removeExactRevertReplacement(root, replacement); err != nil {
			return err
		}
		return removeExactRevertTransactionDir(root, transactionDir)
	} else if err != nil {
		return fmt.Errorf("stat exact revert quarantine: %w", err)
	}
	if !req.After.Exists {
		return ErrExactRevertPending
	}
	if err := verifyExactRootFileState(root, quarantine, req.After); err != nil {
		return fmt.Errorf("%w: verify pending quarantine: %w", ErrExactRevertPending, err)
	}

	beforeMatches := verifyExactRootFileState(root, relative, req.Before) == nil
	afterMatches := verifyExactRootFileState(root, relative, req.After) == nil
	absent := verifyExactRootFileState(root, relative, ExactFileState{}) == nil
	switch {
	case beforeMatches || afterMatches:
		if err := root.Remove(quarantine); err != nil {
			return fmt.Errorf("remove recovered exact revert quarantine: %w", err)
		}
	case absent && req.Before.Exists:
		if err := restoreQuarantinedRootFile(root, quarantine, relative); err != nil {
			return fmt.Errorf("restore interrupted exact revert quarantine; preserved at %q: %w", quarantine, err)
		}
	case absent:
		if err := root.Remove(quarantine); err != nil {
			return fmt.Errorf("remove completed exact revert quarantine: %w", err)
		}
	default:
		return fmt.Errorf("%w: target and quarantine states conflict", ErrExactRevertPending)
	}
	if err := removeExactRevertReplacement(root, replacement); err != nil {
		return err
	}
	return removeExactRevertTransactionDir(root, transactionDir)
}

func verifyExactFileState(path string, expected ExactFileState) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if expected.Exists {
			return ErrExactStateMismatch
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat exact file state: %w", err)
	}
	if info.Mode()&iofs.ModeSymlink != 0 {
		return ErrExactRevertSymlink
	}
	if !info.Mode().IsRegular() {
		return ErrExactRevertUnsupported
	}
	if !expected.Exists {
		return ErrExactStateMismatch
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read exact file state: %w", err)
	}
	if exactSHA256(content) != expected.SHA256 {
		return ErrExactStateMismatch
	}
	if expected.CheckMode && exactFileMode(info.Mode()) != exactFileMode(expected.Mode) {
		return ErrExactStateMismatch
	}
	return nil
}

func rejectSymlinkComponents(root, path string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return fmt.Errorf("relativize exact revert path: %w", err)
	}
	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("stat exact revert path component: %w", err)
		}
		if info.Mode()&iofs.ModeSymlink != 0 {
			return ErrExactRevertSymlink
		}
	}
	return nil
}

func rejectRootSymlinkComponents(root *os.Root, path string) error {
	current := "."
	for _, part := range strings.Split(filepath.Clean(path), string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := root.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("stat rooted revert path component: %w", err)
		}
		if info.Mode()&iofs.ModeSymlink != 0 {
			return ErrExactRevertSymlink
		}
	}
	return nil
}

func verifyExactRootFileState(root *os.Root, path string, expected ExactFileState) error {
	info, err := root.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if expected.Exists {
			return ErrExactStateMismatch
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat rooted exact file state: %w", err)
	}
	if info.Mode()&iofs.ModeSymlink != 0 {
		return ErrExactRevertSymlink
	}
	if !info.Mode().IsRegular() {
		return ErrExactRevertUnsupported
	}
	if !expected.Exists {
		return ErrExactStateMismatch
	}
	content, err := root.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read rooted exact file state: %w", err)
	}
	if exactSHA256(content) != expected.SHA256 {
		return ErrExactStateMismatch
	}
	if expected.CheckMode && exactFileMode(info.Mode()) != exactFileMode(expected.Mode) {
		return ErrExactStateMismatch
	}
	return nil
}

func atomicWriteRootFile(
	root *os.Root,
	path string,
	content []byte,
	mode iofs.FileMode,
	expectedAfter ExactFileState,
	transactionID string,
) (err error) {
	transactionDir, quarantine, replacement, err := beginExactRevertTransaction(root, path, transactionID)
	if err != nil {
		return err
	}
	temp, err := root.OpenFile(replacement, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		_ = removeExactRevertTransactionDir(root, transactionDir)
		return fmt.Errorf("create rooted revert temp file: %w", err)
	}
	defer func() {
		_ = temp.Close()
		if err != nil {
			_ = removeExactRevertReplacement(root, replacement)
			_ = removeExactRevertTransactionDir(root, transactionDir)
		}
	}()
	if _, err = temp.Write(content); err != nil {
		return fmt.Errorf("write revert temp file: %w", err)
	}
	if err = temp.Sync(); err != nil {
		return fmt.Errorf("sync revert temp file: %w", err)
	}
	if err = temp.Chmod(exactFileMode(mode)); err != nil {
		return fmt.Errorf("set reverted file mode: %w", err)
	}
	if err = temp.Close(); err != nil {
		return fmt.Errorf("close revert temp file: %w", err)
	}
	if expectedAfter.Exists {
		if err = quarantineExactRootFileInTransaction(root, path, quarantine, expectedAfter); err != nil {
			return err
		}
		if err = installRootReplacement(root, replacement, path, quarantine); err != nil {
			return err
		}
	} else if err = verifyExactRootFileState(root, path, expectedAfter); err != nil {
		return err
	} else if err = root.Link(replacement, path); err != nil {
		return fmt.Errorf("install reverted file without replacement: %w", err)
	}
	if err = root.Remove(replacement); err != nil {
		return fmt.Errorf("remove exact revert replacement link: %w", err)
	}
	if err = removeExactRevertTransactionDir(root, transactionDir); err != nil {
		return err
	}
	return nil
}

func removeExactRootFile(root *os.Root, path string, expectedAfter ExactFileState, transactionID string) error {
	quarantine, transactionDir, err := quarantineExactRootFile(root, path, expectedAfter, transactionID)
	if err != nil {
		return err
	}
	if err := removeQuarantinedRootFile(root, quarantine, path); err != nil {
		_ = removeExactRevertTransactionDir(root, transactionDir)
		return err
	}
	return removeExactRevertTransactionDir(root, transactionDir)
}

func removeQuarantinedRootFile(root exactRootLinkRemover, quarantine, path string) error {
	if err := root.Remove(quarantine); err != nil {
		removeErr := fmt.Errorf("remove quarantined reverted file: %w", err)
		if restoreErr := restoreQuarantinedRootFile(root, quarantine, path); restoreErr != nil {
			return errors.Join(removeErr, fmt.Errorf("restore file after failed removal; preserved at %q: %w", quarantine, restoreErr))
		}
		return removeErr
	}
	return nil
}

type exactRootLinkRemover interface {
	Link(oldname, newname string) error
	Remove(name string) error
}

func installRootReplacement(root exactRootLinkRemover, tempPath, path, quarantine string) error {
	if err := root.Link(tempPath, path); err != nil {
		installErr := fmt.Errorf("install reverted file without replacement: %w", err)
		if restoreErr := restoreQuarantinedRootFile(root, quarantine, path); restoreErr != nil {
			return errors.Join(installErr, fmt.Errorf("restore file after failed installation; preserved at %q: %w", quarantine, restoreErr))
		}
		return installErr
	}
	if err := root.Remove(quarantine); err != nil {
		cleanupErr := fmt.Errorf("remove replaced file quarantine: %w", err)
		if removeErr := root.Remove(path); removeErr != nil {
			return errors.Join(cleanupErr, fmt.Errorf("remove installed file during rollback; preserved prior file at %q: %w", quarantine, removeErr))
		}
		if restoreErr := restoreQuarantinedRootFile(root, quarantine, path); restoreErr != nil {
			return errors.Join(cleanupErr, fmt.Errorf("restore prior file after cleanup failure; preserved at %q: %w", quarantine, restoreErr))
		}
		return cleanupErr
	}
	return nil
}

func quarantineExactRootFile(root *os.Root, path string, expected ExactFileState, transactionID string) (string, string, error) {
	transactionDir, quarantine, _, err := beginExactRevertTransaction(root, path, transactionID)
	if err != nil {
		return "", transactionDir, err
	}
	if err := quarantineExactRootFileInTransaction(root, path, quarantine, expected); err != nil {
		_ = removeExactRevertTransactionDir(root, transactionDir)
		return "", transactionDir, err
	}
	return quarantine, transactionDir, nil
}

func quarantineExactRootFileInTransaction(root *os.Root, path, quarantine string, expected ExactFileState) error {
	if err := root.Rename(path, quarantine); err != nil {
		return fmt.Errorf("quarantine current file: %w", err)
	}
	if err := verifyExactRootFileState(root, quarantine, expected); err != nil {
		if restoreErr := restoreQuarantinedRootFile(root, quarantine, path); restoreErr != nil {
			return fmt.Errorf("%w; preserve concurrently changed file at %q: %w", err, quarantine, restoreErr)
		}
		return err
	}
	return nil
}

func restoreQuarantinedRootFile(root exactRootLinkRemover, quarantine, path string) error {
	if err := root.Link(quarantine, path); err != nil {
		return err
	}
	if err := root.Remove(quarantine); err != nil {
		return fmt.Errorf("remove restored quarantine link: %w", err)
	}
	return nil
}

func beginExactRevertTransaction(root *os.Root, path, transactionID string) (string, string, string, error) {
	transactionDir, quarantine, replacement := exactRevertTransactionPaths(path, transactionID)
	if err := root.Mkdir(transactionDir, 0o700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return transactionDir, quarantine, replacement, ErrExactRevertPending
		}
		return transactionDir, quarantine, replacement, fmt.Errorf("create exact revert transaction: %w", err)
	}
	return transactionDir, quarantine, replacement, nil
}

func exactRevertTransactionPaths(path, transactionID string) (string, string, string) {
	sum := sha256.Sum256([]byte(filepath.ToSlash(filepath.Clean(path)) + "\x00" + transactionID))
	transactionDir := filepath.Join(
		filepath.Dir(path),
		".gollem-revert-"+hex.EncodeToString(sum[:16]),
	)
	return transactionDir, filepath.Join(transactionDir, "current"), filepath.Join(transactionDir, "replacement")
}

func removeExactRevertReplacement(root *os.Root, replacement string) error {
	info, err := root.Lstat(replacement)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat exact revert replacement: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&iofs.ModeSymlink != 0 {
		return ErrExactRevertPending
	}
	if err := root.Remove(replacement); err != nil {
		return fmt.Errorf("remove exact revert replacement: %w", err)
	}
	return nil
}

func removeExactRevertTransactionDir(root *os.Root, transactionDir string) error {
	if err := root.Remove(transactionDir); err != nil {
		return fmt.Errorf("remove exact revert transaction: %w", err)
	}
	return nil
}

func unusedRootPath(root *os.Root, parent, prefix string) (string, error) {
	for range 16 {
		var suffix [12]byte
		if _, err := rand.Read(suffix[:]); err != nil {
			return "", fmt.Errorf("create revert temporary name: %w", err)
		}
		path := filepath.Join(parent, prefix+hex.EncodeToString(suffix[:]))
		if _, err := root.Lstat(path); errors.Is(err, os.ErrNotExist) {
			return path, nil
		} else if err != nil {
			return "", fmt.Errorf("check revert temporary path: %w", err)
		}
	}
	return "", errors.New("appserver/fs: could not allocate revert temporary path")
}

func exactSHA256(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func exactFileMode(mode iofs.FileMode) iofs.FileMode {
	return mode & exactFileModeMask
}

func isSymlink(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&iofs.ModeSymlink != 0
}

func (s *Service) resolve(path string) (string, error) {
	if s == nil {
		return "", errors.New("appserver/fs: nil service")
	}
	if path == "" {
		path = "."
	}
	var candidate string
	if filepath.IsAbs(path) {
		candidate = filepath.Clean(path)
	} else {
		candidate = filepath.Join(s.root, path)
	}
	if err := ensureInside(s.root, candidate); err != nil {
		return "", err
	}
	eval, err := evalExistingOrParent(candidate)
	if err != nil {
		return "", err
	}
	if err := ensureInside(s.rootEval, eval); err != nil {
		return "", err
	}
	return candidate, nil
}

func evalExistingOrParent(path string) (string, error) {
	if eval, err := filepath.EvalSymlinks(path); err == nil {
		return eval, nil
	}
	var missing []string
	current := filepath.Clean(path)
	for {
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("evaluate existing ancestor: %w", os.ErrNotExist)
		}
		missing = append([]string{filepath.Base(current)}, missing...)
		evalParent, err := filepath.EvalSymlinks(parent)
		if err == nil {
			eval := evalParent
			for _, part := range missing {
				eval = filepath.Join(eval, part)
			}
			return eval, nil
		}
		current = parent
	}
}

func ensureInside(root, path string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return fmt.Errorf("relativize path: %w", err)
	}
	if rel == "." {
		return nil
	}
	if strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return ErrPathOutsideRoot
	}
	return nil
}

func (s *Service) rel(path string) string {
	rel, err := filepath.Rel(s.root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func (s *Service) resolveMutationOperation(op Operation) (string, string, error) {
	switch op.Kind {
	case OperationWriteFile, OperationCreateDirectory, OperationRemove:
	case OperationCopy:
		if strings.TrimSpace(op.Destination) == "" {
			return "", "", errors.New("appserver/fs: mutation destination is required")
		}
	default:
		return "", "", ErrInvalidMutationScope
	}
	resolved, err := s.resolve(op.Path)
	if err != nil {
		return "", "", err
	}
	if op.Kind != OperationCopy {
		return resolved, "", nil
	}
	destination, err := s.resolve(op.Destination)
	if err != nil {
		return resolved, "", err
	}
	return resolved, destination, nil
}

func (s *Service) enterMutation(ctx context.Context, op Operation) (func(), bool, error) {
	var scope *approvedMutationScope
	if ctx != nil {
		scope, _ = ctx.Value(approvedMutationScopeKey{}).(*approvedMutationScope)
	}
	if scope != nil && scope.service == s {
		scope.mu.Lock()
		defer scope.mu.Unlock()
		if !scope.active || scope.used || scope.operation != op {
			return func() {}, false, ErrInvalidMutationScope
		}
		scope.used = true
		return func() {}, true, nil
	}
	s.mutationMu.Lock()
	return s.mutationMu.Unlock, false, nil
}

func (s *Service) requireApproval(ctx context.Context, op Operation) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	if s.approve == nil {
		return nil
	}
	if err := s.approve(ctx, op); err != nil {
		return fmt.Errorf("%w: %w", ErrApprovalDenied, err)
	}
	return nil
}

func (s *Service) emit(op Operation, resolved, dst string, allowed bool, err error) {
	if s == nil || s.audit == nil {
		return
	}
	event := AuditEvent{
		Operation:   op,
		Resolved:    resolved,
		Destination: dst,
		Allowed:     allowed,
		At:          time.Now().UTC(),
	}
	if err != nil {
		event.Err = err.Error()
	}
	s.audit(event)
}

func checkContext(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func samePath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}

func pathInside(root, path string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." && !filepath.IsAbs(rel)
}

func copyFile(src, dst string, mode iofs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create destination parent: %w", err)
	}
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return fmt.Errorf("open destination: %w", err)
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return fmt.Errorf("copy file: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close destination: %w", closeErr)
	}
	return nil
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d iofs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("relativize copy path: %w", err)
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat copy source: %w", err)
		}
		if d.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if info.Mode().Type() != 0 {
			return nil
		}
		return copyFile(path, target, info.Mode())
	})
}
