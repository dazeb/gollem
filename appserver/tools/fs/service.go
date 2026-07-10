package fs

import (
	"context"
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
	ErrWatchPathNotAbsolute   = errors.New("appserver/fs: watch path must be absolute")
	ErrWatchIDRequired        = errors.New("appserver/fs: watch id is required")
	ErrWatchAlreadyExists     = errors.New("appserver/fs: watch id already exists")
	ErrWatchNotFound          = errors.New("appserver/fs: watch id not found")
)

type OperationKind string

const (
	OperationReadFile        OperationKind = "readFile"
	OperationWriteFile       OperationKind = "writeFile"
	OperationCreateDirectory OperationKind = "createDirectory"
	OperationReadDirectory   OperationKind = "readDirectory"
	OperationMetadata        OperationKind = "getMetadata"
	OperationRemove          OperationKind = "remove"
	OperationCopy            OperationKind = "copy"
	OperationWatch           OperationKind = "watch"
	OperationUnwatch         OperationKind = "unwatch"
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
	Size    int64
	Mode    iofs.FileMode
	ModTime time.Time
}

type Metadata struct {
	Path    string
	IsDir   bool
	Size    int64
	Mode    iofs.FileMode
	ModTime time.Time
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
	if err := checkContext(ctx); err != nil {
		s.emit(op, "", "", false, err)
		return err
	}
	resolved, err := s.resolve(path)
	if err != nil {
		s.emit(op, "", "", false, err)
		return err
	}
	if err := s.requireApproval(ctx, op); err != nil {
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
	op := Operation{Kind: OperationCreateDirectory, Path: path}
	if err := checkContext(ctx); err != nil {
		s.emit(op, "", "", false, err)
		return err
	}
	resolved, err := s.resolve(path)
	if err != nil {
		s.emit(op, "", "", false, err)
		return err
	}
	if err := s.requireApproval(ctx, op); err != nil {
		s.emit(op, resolved, "", false, err)
		return err
	}
	if err := os.MkdirAll(resolved, 0o755); err != nil {
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
		Path:    s.rel(resolved),
		IsDir:   info.IsDir(),
		Size:    info.Size(),
		Mode:    info.Mode(),
		ModTime: info.ModTime(),
	}, nil
}

func (s *Service) Remove(ctx context.Context, path string) error {
	op := Operation{Kind: OperationRemove, Path: path, Destructive: true}
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
	if err := s.requireApproval(ctx, op); err != nil {
		s.emit(op, resolved, "", false, err)
		return err
	}
	if err := os.RemoveAll(resolved); err != nil {
		s.emit(op, resolved, "", false, err)
		return fmt.Errorf("remove path: %w", err)
	}
	s.emit(op, resolved, "", true, nil)
	return nil
}

func (s *Service) Copy(ctx context.Context, src, dst string) error {
	op := Operation{Kind: OperationCopy, Path: src, Destination: dst}
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
	if err := s.requireApproval(ctx, op); err != nil {
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
