package team

import "time"

// MessageType classifies a team message.
type MessageType string

const (
	MessageText            MessageType = "text"
	MessageTaskAssignment  MessageType = "task_assignment"
	MessageShutdownRequest MessageType = "shutdown_request"
	MessageStatusUpdate    MessageType = "status_update"
)

// Message is a communication unit between teammates.
type Message struct {
	From      string      `json:"from"`
	To        string      `json:"to"`
	Type      MessageType `json:"type"`
	Content   string      `json:"content"`
	Summary   string      `json:"summary,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
}

// Mailbox is a buffered channel-based message queue for a teammate.
type Mailbox struct {
	ch chan Message
}

// NewMailbox creates a mailbox with the given buffer size.
func NewMailbox(bufferSize int) *Mailbox {
	return &Mailbox{ch: make(chan Message, bufferSize)}
}

// Send enqueues a message without blocking. If the buffer is full,
// the message is dropped (shouldn't happen with reasonable buffer sizes).
func (m *Mailbox) Send(msg Message) {
	select {
	case m.ch <- msg:
	default:
		// Buffer full — drop message rather than blocking the sender.
	}
}

// DrainAll reads all pending messages without blocking.
func (m *Mailbox) DrainAll() []Message {
	var msgs []Message
	for {
		select {
		case msg := <-m.ch:
			msgs = append(msgs, msg)
		default:
			return msgs
		}
	}
}

// Receive returns the underlying channel for select-based reads.
func (m *Mailbox) Receive() <-chan Message {
	return m.ch
}

// Len returns the number of pending messages.
func (m *Mailbox) Len() int {
	return len(m.ch)
}
