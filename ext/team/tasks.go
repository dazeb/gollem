package team

import (
	"fmt"
	"strings"

	"github.com/fugue-labs/gollem/ext/orchestrator"
)

type teamTaskView struct {
	ID          string                   `json:"id"`
	Subject     string                   `json:"subject"`
	Description string                   `json:"description,omitempty"`
	Status      orchestrator.TaskStatus  `json:"status"`
	Assignee    string                   `json:"assignee,omitempty"`
	Attempt     int                      `json:"attempt,omitempty"`
	LastError   string                   `json:"last_error,omitempty"`
	Run         *orchestrator.RunRef     `json:"run,omitempty"`
	Result      *orchestrator.TaskResult `json:"result,omitempty"`
	CreatedAt   string                   `json:"created_at,omitempty"`
	UpdatedAt   string                   `json:"updated_at,omitempty"`
	StartedAt   string                   `json:"started_at,omitempty"`
	CompletedAt string                   `json:"completed_at,omitempty"`
}

func buildTeamTaskPrompt(subject, description string) string {
	subject = strings.TrimSpace(subject)
	description = strings.TrimSpace(description)
	switch {
	case subject == "" && description == "":
		return ""
	case description == "":
		return subject
	case subject == "":
		return description
	default:
		return fmt.Sprintf("Task: %s\n\nDetails:\n%s", subject, description)
	}
}

func teamTaskPrompt(task *orchestrator.Task) string {
	if task == nil {
		return ""
	}
	if strings.TrimSpace(task.Input) != "" {
		return task.Input
	}
	return buildTeamTaskPrompt(task.Subject, task.Description)
}

func newTeamTaskMetadata(teamName, assignee, createdBy string) map[string]any {
	metadata := map[string]any{
		teamMetadataName: teamName,
	}
	if assignee != "" {
		metadata[teamMetadataAssignee] = assignee
	}
	if createdBy != "" {
		metadata[teamMetadataCreatedBy] = createdBy
	}
	return metadata
}

func teamTaskName(task *orchestrator.Task) string {
	return metadataString(task, teamMetadataName)
}

func teamTaskAssignee(task *orchestrator.Task) string {
	return metadataString(task, teamMetadataAssignee)
}

func metadataString(task *orchestrator.Task, key string) string {
	if task == nil || task.Metadata == nil {
		return ""
	}
	value, ok := task.Metadata[key]
	if !ok {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprint(value)
}

func taskView(task *orchestrator.Task) teamTaskView {
	view := teamTaskView{
		ID:          task.ID,
		Subject:     task.Subject,
		Description: task.Description,
		Status:      task.Status,
		Assignee:    teamTaskAssignee(task),
		Attempt:     task.Attempt,
		LastError:   task.LastError,
		Run:         task.Run,
		Result:      task.Result,
	}
	if !task.CreatedAt.IsZero() {
		view.CreatedAt = task.CreatedAt.UTC().Format(timeFormatRFC3339Nano)
	}
	if !task.UpdatedAt.IsZero() {
		view.UpdatedAt = task.UpdatedAt.UTC().Format(timeFormatRFC3339Nano)
	}
	if !task.StartedAt.IsZero() {
		view.StartedAt = task.StartedAt.UTC().Format(timeFormatRFC3339Nano)
	}
	if !task.CompletedAt.IsZero() {
		view.CompletedAt = task.CompletedAt.UTC().Format(timeFormatRFC3339Nano)
	}
	return view
}

func cloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(src))
	for key, value := range src {
		cloned[key] = value
	}
	return cloned
}

const timeFormatRFC3339Nano = "2006-01-02T15:04:05.999999999Z07:00"
