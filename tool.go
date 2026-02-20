package gollem

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
)

// ToolKind classifies tool types.
type ToolKind string

const (
	// ToolKindFunction is a regular callable tool.
	ToolKindFunction ToolKind = "function"
	// ToolKindOutput is a synthetic tool used to extract structured output.
	ToolKindOutput ToolKind = "output"
)

// ToolDefinition describes a tool for the model.
type ToolDefinition struct {
	Name             string `json:"name"`
	Description      string `json:"description,omitempty"`
	ParametersSchema Schema `json:"parameters_schema"`
	Kind             ToolKind `json:"kind"`
	Strict           *bool  `json:"strict,omitempty"`
	Sequential       bool   `json:"sequential,omitempty"`
	OuterTypedDictKey string `json:"outer_typed_dict_key,omitempty"`
}

// RunContext provides tools with access to agent run state.
type RunContext struct {
	Deps       any            // user-provided dependencies
	Usage      RunUsage       // current usage
	Prompt     string         // the user prompt
	Retry      int            // current retry count for this tool
	MaxRetries int            // max retries configured
	ToolName   string         // name of the current tool
	ToolCallID string         // ID of the current tool call
	Messages   []ModelMessage // conversation history (read-only)
	RunStep    int            // current step number
	RunID      string         // unique run ID
}

// ToolHandler is the function that executes a tool.
type ToolHandler func(ctx context.Context, rc *RunContext, argsJSON string) (any, error)

// Tool is a registered tool with its definition and handler.
type Tool struct {
	Definition ToolDefinition
	Handler    ToolHandler
	MaxRetries *int // nil = use agent default
}

// ToolOption configures a tool via functional options.
type ToolOption func(*toolConfig)

type toolConfig struct {
	maxRetries *int
	sequential bool
	strict     *bool
}

// WithToolMaxRetries sets the maximum retries for a tool.
func WithToolMaxRetries(n int) ToolOption {
	return func(c *toolConfig) {
		c.maxRetries = &n
	}
}

// WithToolSequential marks a tool as requiring sequential execution.
func WithToolSequential(seq bool) ToolOption {
	return func(c *toolConfig) {
		c.sequential = seq
	}
}

// WithToolStrict enables strict JSON Schema validation for the tool.
func WithToolStrict(strict bool) ToolOption {
	return func(c *toolConfig) {
		c.strict = &strict
	}
}

var (
	contextType    = reflect.TypeOf((*context.Context)(nil)).Elem()
	runContextType = reflect.TypeOf((*RunContext)(nil))
	errorType      = reflect.TypeOf((*error)(nil)).Elem()
)

// FuncTool creates a Tool from a typed Go function using reflection for
// schema generation. The function must have one of these signatures:
//
//	func(ctx context.Context, params P) (R, error)
//	func(ctx context.Context, rc *RunContext, params P) (R, error)
//
// P is the parameters struct type whose fields are converted to a JSON Schema.
// R is the return type that will be serialized to JSON.
func FuncTool[P any](name, description string, fn any, opts ...ToolOption) Tool {
	cfg := &toolConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	fnVal := reflect.ValueOf(fn)
	fnType := fnVal.Type()

	if fnType.Kind() != reflect.Func {
		panic(fmt.Sprintf("gollem.FuncTool: expected function, got %T", fn))
	}

	// Determine function signature pattern.
	takesRunContext := false
	paramIdx := 1 // index of the params argument

	numIn := fnType.NumIn()
	if numIn < 2 || numIn > 3 {
		panic(fmt.Sprintf("gollem.FuncTool: function must have 2 or 3 parameters, got %d", numIn))
	}
	if !fnType.In(0).Implements(contextType) {
		panic("gollem.FuncTool: first parameter must be context.Context")
	}
	if numIn == 3 {
		if fnType.In(1) != runContextType {
			panic("gollem.FuncTool: second parameter must be *RunContext when 3 parameters are given")
		}
		takesRunContext = true
		paramIdx = 2
	}
	if fnType.NumOut() != 2 {
		panic(fmt.Sprintf("gollem.FuncTool: function must return (R, error), got %d return values", fnType.NumOut()))
	}
	if !fnType.Out(1).Implements(errorType) {
		panic("gollem.FuncTool: second return value must implement error")
	}

	// Generate JSON schema from the parameter type.
	paramsType := fnType.In(paramIdx)
	// Dereference pointer if needed.
	actualParamsType := paramsType
	for actualParamsType.Kind() == reflect.Ptr {
		actualParamsType = actualParamsType.Elem()
	}
	schema := schemaForType(actualParamsType, make(map[reflect.Type]bool))

	def := ToolDefinition{
		Name:             name,
		Description:      description,
		ParametersSchema: schema,
		Kind:             ToolKindFunction,
		Strict:           cfg.strict,
		Sequential:       cfg.sequential,
	}

	handler := func(ctx context.Context, rc *RunContext, argsJSON string) (any, error) {
		// Deserialize args into the parameter type.
		paramVal := reflect.New(actualParamsType)
		if argsJSON != "" && argsJSON != "{}" {
			if err := json.Unmarshal([]byte(argsJSON), paramVal.Interface()); err != nil {
				return nil, fmt.Errorf("failed to unmarshal tool arguments: %w", err)
			}
		}

		// Build function arguments.
		args := make([]reflect.Value, numIn)
		args[0] = reflect.ValueOf(ctx)
		if takesRunContext {
			args[1] = reflect.ValueOf(rc)
		}
		// Pass the param value (dereferenced if the function expects a value type).
		if paramsType.Kind() == reflect.Ptr {
			args[paramIdx] = paramVal
		} else {
			args[paramIdx] = paramVal.Elem()
		}

		// Call the function.
		results := fnVal.Call(args)

		// Extract return values.
		resultVal := results[0].Interface()
		errVal := results[1].Interface()
		if errVal != nil {
			return nil, errVal.(error)
		}
		return resultVal, nil
	}

	return Tool{
		Definition: def,
		Handler:    handler,
		MaxRetries: cfg.maxRetries,
	}
}
