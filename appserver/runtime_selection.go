package appserver

import "github.com/fugue-labs/gollem/core"

func (s *Server) validateRuntimeSelection(selection RuntimeModelSelection, settings core.ModelSettings) error {
	if s != nil && s.selectionValidator != nil {
		if err := s.selectionValidator(selection); err != nil {
			return err
		}
	}
	return validateRuntimeReasoningSelection(s.catalog, selection, settings)
}
