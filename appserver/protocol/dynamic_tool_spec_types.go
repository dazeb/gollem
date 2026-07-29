package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// DynamicToolFunctionSpec is one standalone function declaration. Exporting
// it does not register, validate, defer, or execute a tool.
type DynamicToolFunctionSpec struct {
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	InputSchema  JsonValue `json:"inputSchema"`
	DeferLoading bool      `json:"deferLoading,omitempty"`
}

func (s DynamicToolFunctionSpec) MarshalJSON() ([]byte, error) {
	return marshalDynamicToolFunctionSpec("", s)
}

func (s *DynamicToolFunctionSpec) UnmarshalJSON(data []byte) error {
	if s == nil {
		return errors.New("decode dynamic-tool function spec into nil receiver")
	}
	const objectName = "dynamic-tool function spec"
	payload, err := decodeRustSerdeObject(
		data, objectName, "name", "description", "inputSchema", "deferLoading",
	)
	if err != nil {
		return err
	}
	value, err := decodeDynamicToolFunctionSpec(payload, objectName)
	if err != nil {
		return err
	}
	*s = value
	return nil
}

// DynamicToolNamespaceTool is the only tool variant accepted inside a public
// namespace. Nested namespace declarations are intentionally impossible.
type DynamicToolNamespaceTool struct {
	Function DynamicToolFunctionSpec
}

func (t DynamicToolNamespaceTool) MarshalJSON() ([]byte, error) {
	return marshalDynamicToolFunctionSpec("function", t.Function)
}

func (t *DynamicToolNamespaceTool) UnmarshalJSON(data []byte) error {
	if t == nil {
		return errors.New("decode dynamic-tool namespace tool into nil receiver")
	}
	const objectName = "dynamic-tool namespace tool"
	toolType, err := decodeDynamicToolSpecType(data, objectName)
	if err != nil {
		return err
	}
	if toolType != "function" {
		return fmt.Errorf("unknown %s type %q", objectName, toolType)
	}
	payload, err := decodeRustSerdeObject(
		data, objectName, "type", "name", "description", "inputSchema", "deferLoading",
	)
	if err != nil {
		return err
	}
	function, err := decodeDynamicToolFunctionSpec(payload, objectName)
	if err != nil {
		return err
	}
	*t = DynamicToolNamespaceTool{Function: function}
	return nil
}

// DynamicToolNamespaceSpec is one ordered public namespace declaration.
type DynamicToolNamespaceSpec struct {
	Name        string                     `json:"name"`
	Description string                     `json:"description"`
	Tools       []DynamicToolNamespaceTool `json:"tools"`
}

func (s DynamicToolNamespaceSpec) MarshalJSON() ([]byte, error) {
	return marshalDynamicToolNamespaceSpec("", s)
}

func (s *DynamicToolNamespaceSpec) UnmarshalJSON(data []byte) error {
	if s == nil {
		return errors.New("decode dynamic-tool namespace spec into nil receiver")
	}
	const objectName = "dynamic-tool namespace spec"
	payload, err := decodeRustSerdeObject(data, objectName, "name", "description", "tools")
	if err != nil {
		return err
	}
	value, err := decodeDynamicToolNamespaceSpec(payload, objectName)
	if err != nil {
		return err
	}
	*s = value
	return nil
}

// DynamicToolSpec retains exactly one public function or namespace variant.
// It remains separate from Gollem's live dynamic-call contracts.
type DynamicToolSpec struct {
	Function  *DynamicToolFunctionSpec
	Namespace *DynamicToolNamespaceSpec
}

func (s DynamicToolSpec) MarshalJSON() ([]byte, error) {
	switch {
	case s.Function != nil && s.Namespace == nil:
		return marshalDynamicToolFunctionSpec("function", *s.Function)
	case s.Function == nil && s.Namespace != nil:
		return marshalDynamicToolNamespaceSpec("namespace", *s.Namespace)
	case s.Function == nil:
		return nil, errors.New("dynamic-tool spec has no variant")
	default:
		return nil, errors.New("dynamic-tool spec has multiple variants")
	}
}

func (s *DynamicToolSpec) UnmarshalJSON(data []byte) error {
	if s == nil {
		return errors.New("decode dynamic-tool spec into nil receiver")
	}
	const objectName = "dynamic-tool spec"
	specType, err := decodeDynamicToolSpecType(data, objectName)
	if err != nil {
		return err
	}
	switch specType {
	case "function":
		payload, err := decodeRustSerdeObject(
			data, objectName, "type", "name", "description", "inputSchema", "deferLoading",
		)
		if err != nil {
			return err
		}
		function, err := decodeDynamicToolFunctionSpec(payload, objectName)
		if err != nil {
			return err
		}
		*s = DynamicToolSpec{Function: &function}
		return nil
	case "namespace":
		payload, err := decodeRustSerdeObject(data, objectName, "type", "name", "description", "tools")
		if err != nil {
			return err
		}
		namespace, err := decodeDynamicToolNamespaceSpec(payload, objectName)
		if err != nil {
			return err
		}
		*s = DynamicToolSpec{Namespace: &namespace}
		return nil
	default:
		return fmt.Errorf("unknown %s type %q", objectName, specType)
	}
}

func decodeDynamicToolSpecType(data []byte, objectName string) (string, error) {
	payload, err := decodeRustSerdeObject(data, objectName, "type")
	if err != nil {
		return "", err
	}
	return decodeRequiredThreadItemValue[string](payload, objectName, "type")
}

func decodeDynamicToolFunctionSpec(
	payload map[string]json.RawMessage,
	objectName string,
) (DynamicToolFunctionSpec, error) {
	name, err := decodeRequiredThreadItemValue[string](payload, objectName, "name")
	if err != nil {
		return DynamicToolFunctionSpec{}, err
	}
	description, err := decodeRequiredThreadItemValue[string](payload, objectName, "description")
	if err != nil {
		return DynamicToolFunctionSpec{}, err
	}
	inputSchema, err := decodeRequiredThreadItemJSONValue(payload, objectName, "inputSchema")
	if err != nil {
		return DynamicToolFunctionSpec{}, err
	}
	deferLoading := false
	if raw, ok := payload["deferLoading"]; ok {
		if isJSONNull(raw) {
			return DynamicToolFunctionSpec{}, fmt.Errorf(
				"decode %s deferLoading: expected boolean", objectName,
			)
		}
		if err := json.Unmarshal(raw, &deferLoading); err != nil {
			return DynamicToolFunctionSpec{}, fmt.Errorf(
				"decode %s deferLoading: %w", objectName, err,
			)
		}
	}
	return DynamicToolFunctionSpec{
		Name: name, Description: description,
		InputSchema: inputSchema, DeferLoading: deferLoading,
	}, nil
}

func decodeDynamicToolNamespaceSpec(
	payload map[string]json.RawMessage,
	objectName string,
) (DynamicToolNamespaceSpec, error) {
	name, err := decodeRequiredThreadItemValue[string](payload, objectName, "name")
	if err != nil {
		return DynamicToolNamespaceSpec{}, err
	}
	description, err := decodeRequiredThreadItemValue[string](payload, objectName, "description")
	if err != nil {
		return DynamicToolNamespaceSpec{}, err
	}
	tools, err := decodeRequiredThreadItemArray[DynamicToolNamespaceTool](
		payload, objectName, "tools",
	)
	if err != nil {
		return DynamicToolNamespaceSpec{}, err
	}
	return DynamicToolNamespaceSpec{Name: name, Description: description, Tools: tools}, nil
}

func marshalDynamicToolFunctionSpec(
	specType string,
	spec DynamicToolFunctionSpec,
) ([]byte, error) {
	return json.Marshal(struct {
		Type         string    `json:"type,omitempty"`
		Name         string    `json:"name"`
		Description  string    `json:"description"`
		InputSchema  JsonValue `json:"inputSchema"`
		DeferLoading bool      `json:"deferLoading,omitempty"`
	}{
		Type: specType, Name: spec.Name, Description: spec.Description,
		InputSchema: spec.InputSchema, DeferLoading: spec.DeferLoading,
	})
}

func marshalDynamicToolNamespaceSpec(
	specType string,
	spec DynamicToolNamespaceSpec,
) ([]byte, error) {
	if spec.Tools == nil {
		return nil, errors.New("dynamic-tool namespace tools cannot be null")
	}
	return json.Marshal(struct {
		Type        string                     `json:"type,omitempty"`
		Name        string                     `json:"name"`
		Description string                     `json:"description"`
		Tools       []DynamicToolNamespaceTool `json:"tools"`
	}{
		Type: specType, Name: spec.Name, Description: spec.Description, Tools: spec.Tools,
	})
}

func dynamicToolSpecSchemas() map[string]Schema {
	functionProperties := Schema{
		"deferLoading": Schema{"type": "boolean"},
		"description":  Schema{"type": "string"},
		"inputSchema":  true,
		"name":         Schema{"type": "string"},
	}
	functionDefinitionProperties := mergeDynamicToolSchemaProperties(
		functionProperties,
		Schema{"inputSchema": Schema{"$ref": "#/$defs/JsonValue"}},
	)
	namespaceProperties := Schema{
		"description": Schema{"type": "string"},
		"name":        Schema{"type": "string"},
		"tools": Schema{
			"items": Schema{"$ref": "#/$defs/DynamicToolNamespaceTool"},
			"type":  "array",
		},
	}
	return map[string]Schema{
		"DynamicToolFunctionSpec": {
			"type":       "object",
			"properties": functionDefinitionProperties,
			"required":   []string{"description", "inputSchema", "name"},
		},
		"DynamicToolNamespaceTool": {"oneOf": []any{
			dynamicToolSpecVariantSchema(
				"function", "FunctionDynamicToolNamespaceTool", functionProperties,
				[]string{"description", "inputSchema", "name"},
			),
		}},
		"DynamicToolNamespaceSpec": {
			"type":       "object",
			"properties": namespaceProperties,
			"required":   []string{"description", "name", "tools"},
		},
		"DynamicToolSpec": {"oneOf": []any{
			dynamicToolSpecVariantSchema(
				"function", "FunctionDynamicToolSpec", functionProperties,
				[]string{"description", "inputSchema", "name"},
			),
			dynamicToolSpecVariantSchema(
				"namespace", "NamespaceDynamicToolSpec", namespaceProperties,
				[]string{"description", "name", "tools"},
			),
		}},
	}
}

func dynamicToolSpecVariantSchema(
	specType string,
	title string,
	properties Schema,
	required []string,
) Schema {
	typeTitle := title + "Type"
	variantProperties := mergeDynamicToolSchemaProperties(properties, Schema{
		"type": Schema{"enum": []any{specType}, "title": typeTitle, "type": "string"},
	})
	variantRequired := append(append([]string(nil), required...), "type")
	return Schema{
		"properties": variantProperties,
		"required":   variantRequired,
		"title":      title,
		"type":       "object",
	}
}

func mergeDynamicToolSchemaProperties(base, overrides Schema) Schema {
	merged := make(Schema, len(base)+len(overrides))
	for name, value := range base {
		merged[name] = value
	}
	for name, value := range overrides {
		merged[name] = value
	}
	return merged
}

var (
	_ json.Marshaler   = DynamicToolFunctionSpec{}
	_ json.Unmarshaler = (*DynamicToolFunctionSpec)(nil)
	_ json.Marshaler   = DynamicToolNamespaceTool{}
	_ json.Unmarshaler = (*DynamicToolNamespaceTool)(nil)
	_ json.Marshaler   = DynamicToolNamespaceSpec{}
	_ json.Unmarshaler = (*DynamicToolNamespaceSpec)(nil)
	_ json.Marshaler   = DynamicToolSpec{}
	_ json.Unmarshaler = (*DynamicToolSpec)(nil)
)
