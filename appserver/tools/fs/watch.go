package fs

import (
	"context"
	"errors"
	"fmt"
	iofs "io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultWatchInterval = 200 * time.Millisecond
	minWatchInterval     = 20 * time.Millisecond
)

type WatchRequest struct {
	WatchID      string
	Path         string
	PollInterval time.Duration
}

type WatchResult struct {
	WatchID string
	Path    string
}

type WatchEvent struct {
	WatchID      string
	Path         string
	ChangedPaths []string
	At           time.Time
}

type WatchSink func(WatchEvent)

type watchRegistration struct {
	id       string
	path     string
	interval time.Duration
	cancel   context.CancelFunc
	done     chan struct{}
	stopOnce sync.Once
}

type watchSnapshot map[string]watchFileState

type watchFileState struct {
	Exists  bool
	IsDir   bool
	Size    int64
	Mode    iofs.FileMode
	ModTime time.Time
}

func (s *Service) Watch(ctx context.Context, req WatchRequest, sink WatchSink) (*WatchResult, error) {
	op := Operation{Kind: OperationWatch, Path: req.Path}
	if err := checkContext(ctx); err != nil {
		s.emit(op, "", "", false, err)
		return nil, err
	}
	watchID := strings.TrimSpace(req.WatchID)
	if watchID == "" {
		s.emit(op, "", "", false, ErrWatchIDRequired)
		return nil, ErrWatchIDRequired
	}
	if !filepath.IsAbs(req.Path) {
		s.emit(op, "", "", false, ErrWatchPathNotAbsolute)
		return nil, ErrWatchPathNotAbsolute
	}
	resolved, err := s.resolve(req.Path)
	if err != nil {
		s.emit(op, "", "", false, err)
		return nil, err
	}
	initial, err := s.snapshotForWatch(resolved)
	if err != nil {
		s.emit(op, resolved, "", false, err)
		return nil, err
	}
	interval := req.PollInterval
	if interval <= 0 {
		interval = defaultWatchInterval
	}
	if interval < minWatchInterval {
		interval = minWatchInterval
	}

	watchCtx, cancel := context.WithCancel(context.Background())
	reg := &watchRegistration{
		id:       watchID,
		path:     filepath.Clean(resolved),
		interval: interval,
		cancel:   cancel,
		done:     make(chan struct{}),
	}
	s.mu.Lock()
	if s.watches == nil {
		s.watches = make(map[string]*watchRegistration)
	}
	if _, exists := s.watches[watchID]; exists {
		s.mu.Unlock()
		cancel()
		s.emit(op, resolved, "", false, ErrWatchAlreadyExists)
		return nil, ErrWatchAlreadyExists
	}
	s.watches[watchID] = reg
	s.mu.Unlock()

	go reg.run(watchCtx, s, initial, sink)
	s.emit(op, resolved, "", true, nil)
	return &WatchResult{WatchID: watchID, Path: reg.path}, nil
}

func (s *Service) Unwatch(ctx context.Context, watchID string) error {
	op := Operation{Kind: OperationUnwatch, Path: strings.TrimSpace(watchID)}
	if err := checkContext(ctx); err != nil {
		s.emit(op, "", "", false, err)
		return err
	}
	watchID = strings.TrimSpace(watchID)
	if watchID == "" {
		s.emit(op, "", "", false, ErrWatchIDRequired)
		return ErrWatchIDRequired
	}
	s.mu.Lock()
	reg := s.watches[watchID]
	if reg != nil {
		delete(s.watches, watchID)
	}
	s.mu.Unlock()
	if reg == nil {
		s.emit(op, "", "", false, ErrWatchNotFound)
		return ErrWatchNotFound
	}
	reg.stop()
	s.emit(op, reg.path, "", true, nil)
	return nil
}

func (s *Service) Close() error {
	s.mu.Lock()
	regs := make([]*watchRegistration, 0, len(s.watches))
	for id, reg := range s.watches {
		regs = append(regs, reg)
		delete(s.watches, id)
	}
	s.mu.Unlock()
	for _, reg := range regs {
		reg.stop()
	}
	return nil
}

func (r *watchRegistration) stop() {
	if r == nil {
		return
	}
	r.stopOnce.Do(func() {
		r.cancel()
		<-r.done
	})
}

func (r *watchRegistration) run(ctx context.Context, svc *Service, snapshot watchSnapshot, sink WatchSink) {
	defer close(r.done)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			next, err := svc.snapshotForWatch(r.path)
			if err != nil {
				continue
			}
			changed := diffWatchSnapshots(snapshot, next)
			snapshot = next
			if len(changed) == 0 || sink == nil {
				continue
			}
			sink(WatchEvent{
				WatchID:      r.id,
				Path:         r.path,
				ChangedPaths: changed,
				At:           time.Now().UTC(),
			})
		}
	}
}

func (s *Service) snapshotForWatch(path string) (watchSnapshot, error) {
	path = filepath.Clean(path)
	if _, err := s.resolve(path); err != nil {
		if errors.Is(err, ErrPathOutsideRoot) {
			return watchSnapshot{path: {Exists: false}}, nil
		}
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return watchSnapshot{path: {Exists: false}}, nil
		}
		return nil, fmt.Errorf("stat watch path: %w", err)
	}
	snapshot := watchSnapshot{}
	if !info.IsDir() {
		snapshot[path] = fileState(info)
		return snapshot, nil
	}
	err = filepath.WalkDir(path, func(child string, d iofs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, os.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if err := ensureInside(s.root, child); err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		snapshot[filepath.Clean(child)] = fileState(info)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("snapshot watch path: %w", err)
	}
	return snapshot, nil
}

func fileState(info os.FileInfo) watchFileState {
	return watchFileState{
		Exists:  true,
		IsDir:   info.IsDir(),
		Size:    info.Size(),
		Mode:    info.Mode(),
		ModTime: info.ModTime(),
	}
}

func diffWatchSnapshots(prev, next watchSnapshot) []string {
	changed := map[string]struct{}{}
	for path, after := range next {
		if before, ok := prev[path]; !ok || before != after {
			changed[path] = struct{}{}
		}
	}
	for path, before := range prev {
		if before.Exists {
			if after, ok := next[path]; !ok || !after.Exists {
				changed[path] = struct{}{}
			}
		}
	}
	paths := make([]string, 0, len(changed))
	for path := range changed {
		paths = append(paths, filepath.Clean(path))
	}
	sort.Strings(paths)
	return paths
}
