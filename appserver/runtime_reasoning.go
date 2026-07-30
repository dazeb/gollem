package appserver

import (
	"errors"
	"fmt"
	"strings"

	"github.com/fugue-labs/gollem/appserver/catalog"
	"github.com/fugue-labs/gollem/core"
)

func validateRuntimeReasoningSelection(
	catalogService *catalog.Catalog,
	selection RuntimeModelSelection,
	settings core.ModelSettings,
) error {
	if settings.ReasoningEffort == nil {
		return nil
	}
	effort := strings.TrimSpace(*settings.ReasoningEffort)
	if effort == "" {
		return errors.New("reasoning effort must not be empty")
	}
	providerID := strings.TrimSpace(firstNonEmpty(selection.ProviderID, selection.Provider))
	modelName := strings.TrimSpace(selection.Model)
	if providerID == "" {
		return fmt.Errorf("provider capability is unavailable for reasoning effort %q", effort)
	}
	includeHidden := true
	response, err := catalogService.ListModels(catalog.ModelListParams{
		ProviderID:    providerID,
		IncludeHidden: &includeHidden,
	})
	if err != nil {
		return fmt.Errorf("read model capability: %w", err)
	}
	var selected *catalog.Model
	for index := range response.Data {
		model := &response.Data[index]
		if modelName == "" {
			if model.IsDefault {
				selected = model
				break
			}
			continue
		}
		if model.ID == modelName || model.Model == modelName {
			selected = model
			break
		}
	}
	if selected == nil {
		return fmt.Errorf("model capability is unavailable for %q", modelName)
	}
	if !selected.Capabilities.Reasoning {
		return fmt.Errorf("model %q does not advertise reasoning", selected.ID)
	}
	for _, option := range selected.SupportedReasoningEfforts {
		if strings.TrimSpace(option.ReasoningEffort) == effort {
			return nil
		}
	}
	return fmt.Errorf(
		"model %q does not advertise reasoning effort %q",
		selected.ID,
		effort,
	)
}
