package appserver

import (
	"encoding/base64"
	"fmt"

	"github.com/fugue-labs/gollem/appserver/protocol"
	toolprocess "github.com/fugue-labs/gollem/appserver/tools/process"
)

func commandExecOutputDeltaNotification(event toolprocess.OutputEvent) (string, any) {
	return "command/exec/outputDelta", protocol.CommandExecOutputDeltaNotification{
		ProcessID:   event.ID,
		Stream:      protocol.CommandExecOutputStream(event.Stream),
		DeltaBase64: base64.StdEncoding.EncodeToString(event.Data),
		CapReached:  false,
	}
}

func (s *Server) nextCommandExecProcessID() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.commandSeq++
	return fmt.Sprintf("cmd-%d", s.commandSeq)
}

func (s *Server) registerCommandExecProcess(processID string) bool {
	if s == nil || processID == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.commandExec == nil {
		s.commandExec = make(map[string]struct{})
	}
	if _, exists := s.commandExec[processID]; exists {
		return false
	}
	s.commandExec[processID] = struct{}{}
	return true
}

func (s *Server) isCommandExecProcess(processID string) bool {
	if s == nil || processID == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.commandExec[processID]
	return ok
}

func (s *Server) unregisterCommandExecProcess(processID string) {
	if s == nil || processID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.commandExec, processID)
}
