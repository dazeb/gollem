package appserver

import (
	"time"

	appcache "github.com/fugue-labs/gollem/appserver/cache"
	"github.com/fugue-labs/gollem/appserver/protocol"
	"github.com/fugue-labs/gollem/appserver/store"
	toolfs "github.com/fugue-labs/gollem/appserver/tools/fs"
)

type cacheBenchmarkNotificationParams struct {
	Passed        bool                     `json:"passed"`
	TargetHitRate float64                  `json:"targetHitRate"`
	Totals        appcache.ProviderStats   `json:"totals"`
	Providers     []appcache.ProviderStats `json:"providers"`
	At            time.Time                `json:"at"`
}

func (s *Server) publishFileChanged(operation, path, destination string) {
	s.PublishNotification("fs/changed", fileChangedParams{
		Path:        path,
		Destination: destination,
		Operation:   operation,
		At:          time.Now().UTC(),
	})
}

func (s *Server) publishWatchChanged(event toolfs.WatchEvent) {
	s.PublishNotification("fs/changed", fsWatchChangedParams{
		WatchID:      event.WatchID,
		ChangedPaths: append([]string(nil), event.ChangedPaths...),
	})
}

func (s *Server) publishThreadNotification(method string, thread *store.Thread) {
	if thread == nil {
		return
	}
	now := time.Now().UTC()
	record := protocolThreadRecord(thread)
	switch method {
	case "thread/archived":
		s.PublishNotification(method, protocol.ThreadArchivedNotification{
			ThreadID: thread.ID,
			Status:   protocol.ThreadLifecycleStatus(thread.Status),
			Thread:   &record,
			At:       &now,
		})
		return
	case "thread/deleted":
		s.PublishNotification(method, protocol.ThreadDeletedNotification{
			ThreadID: thread.ID,
			Status:   protocol.ThreadLifecycleStatus(thread.Status),
			Thread:   &record,
			At:       &now,
		})
		return
	case "thread/unarchived":
		s.PublishNotification(method, protocol.ThreadUnarchivedNotification{
			ThreadID: thread.ID,
			Status:   protocol.ThreadLifecycleStatus(thread.Status),
			Thread:   &record,
			At:       &now,
		})
		return
	}
	s.PublishNotification(method, threadNotificationParams{
		ThreadID: thread.ID,
		Status:   thread.Status,
		Thread:   thread,
		At:       now,
	})
}

func (s *Server) publishThreadClosed(threadID string) {
	if threadID == "" {
		return
	}
	s.PublishNotification("thread/closed", protocol.ThreadClosedNotification{ThreadID: threadID})
	s.PublishNotification("thread/status/changed", threadNotLoadedStatusNotificationParams{
		ThreadID: threadID,
		Status:   map[string]string{"type": "notLoaded"},
		At:       time.Now().UTC(),
	})
}

func (s *Server) publishThreadGoalUpdated(thread *store.Thread, goal protocol.ThreadGoal, turnID *string) {
	if thread == nil {
		return
	}
	now := time.Now().UTC()
	record := protocolThreadRecord(thread)
	s.PublishNotification("thread/goal/updated", protocol.ThreadGoalUpdatedNotification{
		ThreadID: thread.ID,
		TurnID:   turnID,
		Goal:     goal,
		Thread:   &record,
		At:       &now,
	})
}

func (s *Server) publishThreadGoalCleared(thread *store.Thread) {
	if thread == nil {
		return
	}
	now := time.Now().UTC()
	record := protocolThreadRecord(thread)
	s.PublishNotification("thread/goal/cleared", protocol.ThreadGoalClearedNotification{
		ThreadID: thread.ID,
		Thread:   &record,
		At:       &now,
	})
}

func (s *Server) publishThreadNameNotification(thread *store.Thread) {
	if thread == nil {
		return
	}
	now := time.Now().UTC()
	name := thread.Title
	record := protocolThreadRecord(thread)
	s.PublishNotification("thread/name/updated", protocol.ThreadNameUpdatedNotification{
		ThreadID:   thread.ID,
		ThreadName: &name,
		Name:       thread.Title,
		Thread:     &record,
		At:         &now,
	})
}

func (s *Server) publishCacheBenchmarkCompleted(result appcache.BenchmarkResponse) {
	providers := make([]appcache.ProviderStats, 0, len(result.Providers))
	for _, provider := range result.Providers {
		providers = append(providers, appcache.ProviderStats{
			Provider:      provider.Provider,
			TotalRequests: provider.TotalRequests,
			Hits:          provider.Hits,
			Misses:        provider.Misses,
			HitRate:       provider.HitRate,
		})
	}
	s.PublishNotification("cache/benchmark/completed", cacheBenchmarkNotificationParams{
		Passed:        result.Passed,
		TargetHitRate: result.TargetHitRate,
		Totals:        result.Totals,
		Providers:     providers,
		At:            time.Now().UTC(),
	})
}
