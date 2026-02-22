package team

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/fugue-labs/gollem/core"
)

// TeammateState represents the lifecycle state of a teammate.
type TeammateState int

const (
	TeammateStarting     TeammateState = iota
	TeammateRunning
	TeammateIdle
	TeammateShuttingDown
	TeammateStopped
)

func (s TeammateState) String() string {
	switch s {
	case TeammateStarting:
		return "starting"
	case TeammateRunning:
		return "running"
	case TeammateIdle:
		return "idle"
	case TeammateShuttingDown:
		return "shutting_down"
	case TeammateStopped:
		return "stopped"
	default:
		return "unknown"
	}
}

// TeammateInfo is a snapshot of teammate state for tools/prompts.
type TeammateInfo struct {
	Name  string        `json:"name"`
	State TeammateState `json:"state"`
}

// Teammate represents a worker agent running as a goroutine.
type Teammate struct {
	mu      sync.Mutex
	name    string
	state   TeammateState
	mailbox *Mailbox
	agent   *core.Agent[string]
	cancel  context.CancelFunc
	team    *Team
	wakeCh  chan struct{}
}

// Name returns the teammate's name.
func (tm *Teammate) Name() string {
	return tm.name
}

// State returns the current state.
func (tm *Teammate) State() TeammateState {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.state
}

func (tm *Teammate) setState(s TeammateState) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.state = s
}

// Wake signals the teammate to process new messages.
func (tm *Teammate) Wake() {
	select {
	case tm.wakeCh <- struct{}{}:
	default:
		// Already signaled.
	}
}

// run is the main goroutine loop for a teammate.
func (tm *Teammate) run(ctx context.Context, initialTask string) {
	defer func() {
		tm.setState(TeammateStopped)
		if tm.team.eventBus != nil {
			core.PublishAsync(tm.team.eventBus, TeammateTerminatedEvent{
				TeamName:     tm.team.name,
				TeammateName: tm.name,
				Reason:       "stopped",
			})
		}
		tm.team.wg.Done()
	}()

	prompt := initialTask

	for {
		tm.setState(TeammateRunning)
		fmt.Fprintf(os.Stderr, "[gollem] team:%s teammate:%s running\n", tm.team.name, tm.name)

		result, err := tm.agent.Run(ctx, prompt)
		if err != nil {
			if ctx.Err() != nil {
				// Context cancelled — shut down.
				return
			}
			fmt.Fprintf(os.Stderr, "[gollem] team:%s teammate:%s error: %v\n", tm.team.name, tm.name, err)
		} else {
			// Notify leader of completion via their mailbox.
			if tm.team.leader != "" {
				leaderMB := tm.team.getMailbox(tm.team.leader)
				if leaderMB != nil {
					summary := result.Output
					if len(summary) > 200 {
						summary = summary[:200] + "..."
					}
					leaderMB.Send(Message{
						From:      tm.name,
						To:        tm.team.leader,
						Type:      MessageStatusUpdate,
						Content:   result.Output,
						Summary:   fmt.Sprintf("%s finished current task", tm.name),
						Timestamp: time.Now(),
					})
				}
			}

			fmt.Fprintf(os.Stderr, "[gollem] team:%s teammate:%s completed (tokens: %d in, %d out)\n",
				tm.team.name, tm.name, result.Usage.InputTokens, result.Usage.OutputTokens)
		}

		// Go idle and wait for new work or shutdown.
		tm.setState(TeammateIdle)
		if tm.team.eventBus != nil {
			core.PublishAsync(tm.team.eventBus, TeammateIdleEvent{
				TeamName:     tm.team.name,
				TeammateName: tm.name,
			})
		}
		fmt.Fprintf(os.Stderr, "[gollem] team:%s teammate:%s idle, waiting for messages\n", tm.team.name, tm.name)

		select {
		case <-ctx.Done():
			return
		case <-tm.team.done:
			return
		case <-tm.wakeCh:
			// Drain mailbox and build new prompt.
			msgs := tm.mailbox.DrainAll()
			if len(msgs) == 0 {
				continue
			}

			// Check for shutdown request.
			for _, msg := range msgs {
				if msg.Type == MessageShutdownRequest {
					fmt.Fprintf(os.Stderr, "[gollem] team:%s teammate:%s received shutdown request\n",
						tm.team.name, tm.name)
					return
				}
			}

			// Build prompt from messages.
			prompt = formatMessagesAsPrompt(msgs)
		}
	}
}

func formatMessagesAsPrompt(msgs []Message) string {
	if len(msgs) == 1 {
		return fmt.Sprintf("[Message from %s]: %s", msgs[0].From, msgs[0].Content)
	}
	var result string
	for i, msg := range msgs {
		if i > 0 {
			result += "\n\n"
		}
		result += fmt.Sprintf("[Message from %s]: %s", msg.From, msg.Content)
	}
	return result
}
