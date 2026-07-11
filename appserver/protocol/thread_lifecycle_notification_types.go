package protocol

import "time"

type ThreadArchivedNotification struct {
	ThreadID string                `json:"threadId"`
	Status   ThreadLifecycleStatus `json:"status,omitempty"`
	Thread   *ThreadRecord         `json:"thread,omitempty"`
	At       *time.Time            `json:"at,omitempty"`
}

type ThreadClosedNotification struct {
	ThreadID string `json:"threadId"`
}

type ThreadDeletedNotification struct {
	ThreadID string                `json:"threadId"`
	Status   ThreadLifecycleStatus `json:"status,omitempty"`
	Thread   *ThreadRecord         `json:"thread,omitempty"`
	At       *time.Time            `json:"at,omitempty"`
}

type ThreadUnarchivedNotification struct {
	ThreadID string                `json:"threadId"`
	Status   ThreadLifecycleStatus `json:"status,omitempty"`
	Thread   *ThreadRecord         `json:"thread,omitempty"`
	At       *time.Time            `json:"at,omitempty"`
}
