package gollem

import (
	"reflect"
)

const (
	// DefaultOutputToolName is the name of the synthetic tool used to extract structured output.
	DefaultOutputToolName = "final_result"
	// DefaultOutputToolDescription is the description for the output tool.
	DefaultOutputToolDescription = "The final response which ends this conversation"
)

// OutputSchema determines how to extract structured output from model responses.
type OutputSchema struct {
	// Mode is the output extraction mode.
	Mode OutputMode

	// OutputTools contains the synthetic tool definitions for tool-based output.
	OutputTools []ToolDefinition

	// OutputObject describes the schema for native structured output.
	OutputObject *OutputObjectDefinition

	// AllowsText is true when text output is acceptable (e.g., T is string).
	AllowsText bool

	// OuterTypedDictKey is set when the output type is wrapped in an object.
	OuterTypedDictKey string

	// outputType is the reflect.Type of the output for deserialization.
	outputType reflect.Type
}

// OutputOption configures output behavior.
type OutputOption func(*outputConfig)

type outputConfig struct {
	mode     *OutputMode
	toolName string
	toolDesc string
	strict   *bool
}

// WithOutputMode forces a specific output mode.
func WithOutputMode(mode OutputMode) OutputOption {
	return func(c *outputConfig) {
		c.mode = &mode
	}
}

// WithOutputToolName sets the name of the output extraction tool.
func WithOutputToolName(name string) OutputOption {
	return func(c *outputConfig) {
		c.toolName = name
	}
}

// WithOutputToolDescription sets the description of the output extraction tool.
func WithOutputToolDescription(desc string) OutputOption {
	return func(c *outputConfig) {
		c.toolDesc = desc
	}
}

// buildOutputSchema constructs an OutputSchema for the given type T.
func buildOutputSchema[T any](opts ...OutputOption) *OutputSchema {
	cfg := &outputConfig{
		toolName: DefaultOutputToolName,
		toolDesc: DefaultOutputToolDescription,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	t := reflect.TypeFor[T]()

	// If T is string, use text mode.
	if t.Kind() == reflect.String {
		mode := OutputModeText
		if cfg.mode != nil {
			mode = *cfg.mode
		}
		return &OutputSchema{
			Mode:       mode,
			AllowsText: true,
			outputType: t,
		}
	}

	// For all other types, use tool mode by default.
	mode := OutputModeTool
	if cfg.mode != nil {
		mode = *cfg.mode
	}

	// Generate the schema for T.
	schema := SchemaFor[T]()
	outerKey := ""

	// If the schema is not an object (e.g., T is []string, int), wrap it.
	if !IsObjectSchema(schema) {
		outerKey = "result"
		schema = Schema{
			"type": "object",
			"properties": map[string]any{
				"result": schema,
			},
			"required": []string{"result"},
		}
	}

	toolDef := ToolDefinition{
		Name:              cfg.toolName,
		Description:       cfg.toolDesc,
		ParametersSchema:  schema,
		Kind:              ToolKindOutput,
		Strict:            cfg.strict,
		OuterTypedDictKey: outerKey,
	}

	os := &OutputSchema{
		Mode:              mode,
		OutputTools:       []ToolDefinition{toolDef},
		AllowsText:        false,
		OuterTypedDictKey: outerKey,
		outputType:        t,
	}

	// For native mode, also set the OutputObject.
	if mode == OutputModeNative {
		os.OutputObject = &OutputObjectDefinition{
			Name:        cfg.toolName,
			Description: cfg.toolDesc,
			JSONSchema:  schema,
			Strict:      cfg.strict,
		}
		os.AllowsText = true
	}

	return os
}

// buildModelRequestParams constructs ModelRequestParameters for the agent.
func buildModelRequestParams(tools []Tool, output *OutputSchema) *ModelRequestParameters {
	params := &ModelRequestParameters{
		OutputMode:      output.Mode,
		AllowTextOutput: output.AllowsText,
	}

	// Add function tools.
	for _, t := range tools {
		params.FunctionTools = append(params.FunctionTools, t.Definition)
	}

	// Add output tools.
	params.OutputTools = output.OutputTools

	// Add output object for native mode.
	params.OutputObject = output.OutputObject

	return params
}
