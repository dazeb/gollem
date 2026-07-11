package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

type McpServerElicitationAction string

const (
	McpServerElicitationAccept  McpServerElicitationAction = "accept"
	McpServerElicitationDecline McpServerElicitationAction = "decline"
	McpServerElicitationCancel  McpServerElicitationAction = "cancel"
)

const (
	McpServerElicitationModeForm       = "form"
	McpServerElicitationModeOpenAIForm = "openai/form"
	McpServerElicitationModeURL        = "url"
)

type McpElicitationObjectType string

const McpElicitationObject McpElicitationObjectType = "object"

type McpElicitationStringType string

const McpElicitationString McpElicitationStringType = "string"

type McpElicitationNumberType string

const (
	McpElicitationNumber  McpElicitationNumberType = "number"
	McpElicitationInteger McpElicitationNumberType = "integer"
)

type McpElicitationBooleanType string

const McpElicitationBoolean McpElicitationBooleanType = "boolean"

type McpElicitationArrayType string

const McpElicitationArray McpElicitationArrayType = "array"

type McpElicitationStringFormat string

const (
	McpElicitationFormatEmail    McpElicitationStringFormat = "email"
	McpElicitationFormatURI      McpElicitationStringFormat = "uri"
	McpElicitationFormatDate     McpElicitationStringFormat = "date"
	McpElicitationFormatDateTime McpElicitationStringFormat = "date-time"
)

type McpElicitationConstOption struct {
	Const string `json:"const"`
	Title string `json:"title"`
}

type McpElicitationStringSchema struct {
	Type        McpElicitationStringType    `json:"type"`
	Title       *string                     `json:"title,omitempty" jsonschema:"nonnullable=true"`
	Description *string                     `json:"description,omitempty" jsonschema:"nonnullable=true"`
	MinLength   *uint32                     `json:"minLength,omitempty" jsonschema:"nonnullable=true"`
	MaxLength   *uint32                     `json:"maxLength,omitempty" jsonschema:"nonnullable=true"`
	Format      *McpElicitationStringFormat `json:"format,omitempty" jsonschema:"nonnullable=true"`
	Default     *string                     `json:"default,omitempty" jsonschema:"nonnullable=true"`
}

type McpElicitationNumberSchema struct {
	Type        McpElicitationNumberType `json:"type"`
	Title       *string                  `json:"title,omitempty" jsonschema:"nonnullable=true"`
	Description *string                  `json:"description,omitempty" jsonschema:"nonnullable=true"`
	Minimum     *float64                 `json:"minimum,omitempty" jsonschema:"nonnullable=true"`
	Maximum     *float64                 `json:"maximum,omitempty" jsonschema:"nonnullable=true"`
	Default     *float64                 `json:"default,omitempty" jsonschema:"nonnullable=true"`
}

type McpElicitationBooleanSchema struct {
	Type        McpElicitationBooleanType `json:"type"`
	Title       *string                   `json:"title,omitempty" jsonschema:"nonnullable=true"`
	Description *string                   `json:"description,omitempty" jsonschema:"nonnullable=true"`
	Default     *bool                     `json:"default,omitempty" jsonschema:"nonnullable=true"`
}

type McpElicitationLegacyTitledEnumSchema struct {
	Type        McpElicitationStringType `json:"type"`
	Title       *string                  `json:"title,omitempty" jsonschema:"nonnullable=true"`
	Description *string                  `json:"description,omitempty" jsonschema:"nonnullable=true"`
	Enum        []string                 `json:"enum" jsonschema:"nonnullable=true"`
	EnumNames   []string                 `json:"enumNames,omitempty" jsonschema:"nonnullable=true"`
	Default     *string                  `json:"default,omitempty" jsonschema:"nonnullable=true"`
}

type McpElicitationUntitledSingleSelectEnumSchema struct {
	Type        McpElicitationStringType `json:"type"`
	Title       *string                  `json:"title,omitempty" jsonschema:"nonnullable=true"`
	Description *string                  `json:"description,omitempty" jsonschema:"nonnullable=true"`
	Enum        []string                 `json:"enum" jsonschema:"nonnullable=true"`
	Default     *string                  `json:"default,omitempty" jsonschema:"nonnullable=true"`
}

type McpElicitationTitledSingleSelectEnumSchema struct {
	Type        McpElicitationStringType    `json:"type"`
	Title       *string                     `json:"title,omitempty" jsonschema:"nonnullable=true"`
	Description *string                     `json:"description,omitempty" jsonschema:"nonnullable=true"`
	OneOf       []McpElicitationConstOption `json:"oneOf" jsonschema:"nonnullable=true"`
	Default     *string                     `json:"default,omitempty" jsonschema:"nonnullable=true"`
}

type McpElicitationUntitledEnumItems struct {
	Type McpElicitationStringType `json:"type"`
	Enum []string                 `json:"enum" jsonschema:"nonnullable=true"`
}

type McpElicitationTitledEnumItems struct {
	AnyOf []McpElicitationConstOption `json:"anyOf" jsonschema:"nonnullable=true"`
}

type McpElicitationUntitledMultiSelectEnumSchema struct {
	Type        McpElicitationArrayType         `json:"type"`
	Title       *string                         `json:"title,omitempty" jsonschema:"nonnullable=true"`
	Description *string                         `json:"description,omitempty" jsonschema:"nonnullable=true"`
	MinItems    *uint64                         `json:"minItems,omitempty" jsonschema:"nonnullable=true"`
	MaxItems    *uint64                         `json:"maxItems,omitempty" jsonschema:"nonnullable=true"`
	Items       McpElicitationUntitledEnumItems `json:"items"`
	Default     []string                        `json:"default,omitempty" jsonschema:"nonnullable=true"`
}

type McpElicitationTitledMultiSelectEnumSchema struct {
	Type        McpElicitationArrayType       `json:"type"`
	Title       *string                       `json:"title,omitempty" jsonschema:"nonnullable=true"`
	Description *string                       `json:"description,omitempty" jsonschema:"nonnullable=true"`
	MinItems    *uint64                       `json:"minItems,omitempty" jsonschema:"nonnullable=true"`
	MaxItems    *uint64                       `json:"maxItems,omitempty" jsonschema:"nonnullable=true"`
	Items       McpElicitationTitledEnumItems `json:"items"`
	Default     []string                      `json:"default,omitempty" jsonschema:"nonnullable=true"`
}

// The public elicitation schema contains nested untagged unions. These wrapper
// types retain their validated JSON so generated clients receive exact unions
// without exposing a misleading flattened Go shape.
type McpElicitationPrimitiveSchema struct{ raw json.RawMessage }
type McpElicitationEnumSchema struct{ raw json.RawMessage }
type McpElicitationSingleSelectEnumSchema struct{ raw json.RawMessage }
type McpElicitationMultiSelectEnumSchema struct{ raw json.RawMessage }

type McpElicitationSchema struct {
	SchemaURI  *string                                  `json:"$schema,omitempty" jsonschema:"nonnullable=true"`
	Type       McpElicitationObjectType                 `json:"type"`
	Properties map[string]McpElicitationPrimitiveSchema `json:"properties" jsonschema:"nonnullable=true"`
	Required   []string                                 `json:"required,omitempty" jsonschema:"nonnullable=true"`
}

type McpServerElicitationRequestParams struct {
	ThreadID        string          `json:"threadId"`
	TurnID          *string         `json:"turnId"`
	ServerName      string          `json:"serverName"`
	Mode            string          `json:"mode"`
	Meta            json.RawMessage `json:"_meta"`
	Message         string          `json:"message"`
	RequestedSchema json.RawMessage `json:"requestedSchema,omitempty"`
	URL             string          `json:"url,omitempty"`
	ElicitationID   string          `json:"elicitationId,omitempty"`
	RequestID       string          `json:"requestId,omitempty"`
	ItemID          string          `json:"itemId,omitempty"`
	StartedAtMS     int64           `json:"startedAtMs,omitempty"`
	ServerID        string          `json:"serverId,omitempty"`
	Schema          map[string]any  `json:"schema,omitempty"`
	Metadata        map[string]any  `json:"metadata,omitempty"`
	Reason          string          `json:"reason,omitempty"`
}

type McpServerElicitationRequestResponse struct {
	Action  McpServerElicitationAction `json:"action"`
	Content json.RawMessage            `json:"content"`
	Meta    json.RawMessage            `json:"_meta"`
}

func (s McpElicitationPrimitiveSchema) MarshalJSON() ([]byte, error) {
	return marshalValidatedMcpElicitationUnion(s.raw, validateMcpElicitationPrimitiveJSON)
}

func (s *McpElicitationPrimitiveSchema) UnmarshalJSON(data []byte) error {
	if s == nil {
		return errors.New("decode MCP elicitation primitive schema into nil receiver")
	}
	canonical, err := validateMcpElicitationPrimitiveJSON(data)
	if err != nil {
		return err
	}
	s.raw = canonical
	return nil
}

func (s McpElicitationEnumSchema) MarshalJSON() ([]byte, error) {
	return marshalValidatedMcpElicitationUnion(s.raw, validateMcpElicitationEnumJSON)
}

func (s *McpElicitationEnumSchema) UnmarshalJSON(data []byte) error {
	if s == nil {
		return errors.New("decode MCP elicitation enum schema into nil receiver")
	}
	canonical, err := validateMcpElicitationEnumJSON(data)
	if err != nil {
		return err
	}
	s.raw = canonical
	return nil
}

func (s McpElicitationSingleSelectEnumSchema) MarshalJSON() ([]byte, error) {
	return marshalValidatedMcpElicitationUnion(s.raw, validateMcpElicitationSingleSelectJSON)
}

func (s *McpElicitationSingleSelectEnumSchema) UnmarshalJSON(data []byte) error {
	if s == nil {
		return errors.New("decode MCP elicitation single-select schema into nil receiver")
	}
	canonical, err := validateMcpElicitationSingleSelectJSON(data)
	if err != nil {
		return err
	}
	s.raw = canonical
	return nil
}

func (s McpElicitationMultiSelectEnumSchema) MarshalJSON() ([]byte, error) {
	return marshalValidatedMcpElicitationUnion(s.raw, validateMcpElicitationMultiSelectJSON)
}

func (s *McpElicitationMultiSelectEnumSchema) UnmarshalJSON(data []byte) error {
	if s == nil {
		return errors.New("decode MCP elicitation multi-select schema into nil receiver")
	}
	canonical, err := validateMcpElicitationMultiSelectJSON(data)
	if err != nil {
		return err
	}
	s.raw = canonical
	return nil
}

func (s McpElicitationSchema) MarshalJSON() ([]byte, error) {
	type wire McpElicitationSchema
	data, err := json.Marshal(wire(s))
	if err != nil {
		return nil, err
	}
	if err := validateMcpElicitationSchemaJSON(data); err != nil {
		return nil, err
	}
	return data, nil
}

func (s *McpElicitationSchema) UnmarshalJSON(data []byte) error {
	if s == nil {
		return errors.New("decode MCP elicitation schema into nil receiver")
	}
	if err := validateMcpElicitationSchemaJSON(data); err != nil {
		return err
	}
	type wire McpElicitationSchema
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*s = McpElicitationSchema(decoded)
	return nil
}

func (p McpServerElicitationRequestParams) MarshalJSON() ([]byte, error) {
	type wire McpServerElicitationRequestParams
	if len(p.Meta) == 0 {
		p.Meta = json.RawMessage("null")
	}
	if p.Mode == McpServerElicitationModeForm {
		canonical, err := canonicalMcpElicitationSchema(p.RequestedSchema)
		if err != nil {
			return nil, err
		}
		p.RequestedSchema = canonical
	}
	data, err := json.Marshal(wire(p))
	if err != nil {
		return nil, err
	}
	if err := validateMcpServerElicitationParamsJSON(data); err != nil {
		return nil, err
	}
	return data, nil
}

func (p *McpServerElicitationRequestParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode MCP elicitation request params into nil receiver")
	}
	if err := validateMcpServerElicitationParamsJSON(data); err != nil {
		return err
	}
	type wire McpServerElicitationRequestParams
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if decoded.Mode == McpServerElicitationModeForm {
		canonical, err := canonicalMcpElicitationSchema(decoded.RequestedSchema)
		if err != nil {
			return err
		}
		decoded.RequestedSchema = canonical
	}
	*p = McpServerElicitationRequestParams(decoded)
	return nil
}

func (r McpServerElicitationRequestResponse) MarshalJSON() ([]byte, error) {
	type wire McpServerElicitationRequestResponse
	if len(r.Content) == 0 {
		r.Content = json.RawMessage("null")
	}
	if len(r.Meta) == 0 {
		r.Meta = json.RawMessage("null")
	}
	data, err := json.Marshal(wire(r))
	if err != nil {
		return nil, err
	}
	if err := validateMcpServerElicitationResponseJSON(data); err != nil {
		return nil, err
	}
	return data, nil
}

func (r *McpServerElicitationRequestResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode MCP elicitation response into nil receiver")
	}
	if err := validateMcpServerElicitationResponseJSON(data); err != nil {
		return err
	}
	type wire McpServerElicitationRequestResponse
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*r = McpServerElicitationRequestResponse(decoded)
	return nil
}

func marshalValidatedMcpElicitationUnion(raw json.RawMessage, validate func([]byte) (json.RawMessage, error)) ([]byte, error) {
	if len(raw) == 0 {
		return nil, errors.New("MCP elicitation union has no value")
	}
	canonical, err := validate(raw)
	if err != nil {
		return nil, err
	}
	return canonical, nil
}

func canonicalMcpElicitationSchema(raw json.RawMessage) (json.RawMessage, error) {
	var schema McpElicitationSchema
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, fmt.Errorf("canonicalize MCP elicitation requestedSchema: %w", err)
	}
	canonical, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("encode canonical MCP elicitation requestedSchema: %w", err)
	}
	return canonical, nil
}

func validateMcpElicitationSchemaJSON(data []byte) error {
	object, err := decodeMcpElicitationObject(data, "$schema", "type", "properties", "required")
	if err != nil {
		return fmt.Errorf("decode MCP elicitation schema: %w", err)
	}
	if err := requireMcpElicitationLiteral(object, "type", "object"); err != nil {
		return err
	}
	propertiesRaw, ok := object["properties"]
	if !ok || isMcpElicitationNull(propertiesRaw) {
		return errors.New("MCP elicitation schema requires properties object")
	}
	var properties map[string]json.RawMessage
	if err := json.Unmarshal(propertiesRaw, &properties); err != nil || properties == nil {
		return errors.New("MCP elicitation schema properties must be an object")
	}
	for name, raw := range properties {
		if _, err := validateMcpElicitationPrimitiveJSON(raw); err != nil {
			return fmt.Errorf("invalid MCP elicitation property %q: %w", name, err)
		}
	}
	if err := optionalMcpElicitationString(object, "$schema"); err != nil {
		return err
	}
	if err := optionalMcpElicitationStringArray(object, "required"); err != nil {
		return err
	}
	return nil
}

func validateMcpElicitationPrimitiveJSON(data []byte) (json.RawMessage, error) {
	object, err := decodeMcpElicitationObject(data)
	if err != nil {
		return nil, err
	}
	typeName, err := requiredMcpElicitationString(object, "type")
	if err != nil {
		return nil, err
	}
	switch typeName {
	case "boolean":
		err = validateMcpElicitationBooleanObject(object)
	case "number", "integer":
		err = validateMcpElicitationNumberObject(object)
	case "array":
		err = validateMcpElicitationMultiSelectObject(object)
	case "string":
		if _, ok := object["oneOf"]; ok {
			err = validateMcpElicitationTitledSingleObject(object)
		} else if _, ok := object["enum"]; ok {
			err = validateMcpElicitationStringEnumObject(object, true)
		} else {
			err = validateMcpElicitationStringObject(object)
		}
	default:
		err = fmt.Errorf("unknown MCP elicitation primitive type %q", typeName)
	}
	if err != nil {
		return nil, err
	}
	return canonicalMcpElicitationObject(object)
}

func validateMcpElicitationEnumJSON(data []byte) (json.RawMessage, error) {
	object, err := decodeMcpElicitationObject(data)
	if err != nil {
		return nil, err
	}
	typeName, err := requiredMcpElicitationString(object, "type")
	if err != nil {
		return nil, err
	}
	switch typeName {
	case "string":
		if _, ok := object["oneOf"]; ok {
			err = validateMcpElicitationTitledSingleObject(object)
		} else {
			err = validateMcpElicitationStringEnumObject(object, true)
		}
	case "array":
		err = validateMcpElicitationMultiSelectObject(object)
	default:
		err = fmt.Errorf("MCP elicitation enum cannot use type %q", typeName)
	}
	if err != nil {
		return nil, err
	}
	return canonicalMcpElicitationObject(object)
}

func validateMcpElicitationSingleSelectJSON(data []byte) (json.RawMessage, error) {
	object, err := decodeMcpElicitationObject(data)
	if err != nil {
		return nil, err
	}
	if err := requireMcpElicitationLiteral(object, "type", "string"); err != nil {
		return nil, err
	}
	if _, ok := object["oneOf"]; ok {
		err = validateMcpElicitationTitledSingleObject(object)
	} else {
		err = validateMcpElicitationStringEnumObject(object, false)
	}
	if err != nil {
		return nil, err
	}
	return canonicalMcpElicitationObject(object)
}

func validateMcpElicitationMultiSelectJSON(data []byte) (json.RawMessage, error) {
	object, err := decodeMcpElicitationObject(data)
	if err != nil {
		return nil, err
	}
	if err := validateMcpElicitationMultiSelectObject(object); err != nil {
		return nil, err
	}
	return canonicalMcpElicitationObject(object)
}

func validateMcpElicitationStringObject(object map[string]json.RawMessage) error {
	if err := allowOnlyMcpElicitationFields(object, "type", "title", "description", "minLength", "maxLength", "format", "default"); err != nil {
		return err
	}
	if err := requireMcpElicitationLiteral(object, "type", "string"); err != nil {
		return err
	}
	for _, name := range []string{"title", "description", "default"} {
		if err := optionalMcpElicitationString(object, name); err != nil {
			return err
		}
	}
	for _, name := range []string{"minLength", "maxLength"} {
		if err := optionalMcpElicitationUnsignedInteger(object, name); err != nil {
			return err
		}
	}
	if raw, ok := object["format"]; ok && !isMcpElicitationNull(raw) {
		format, err := decodeMcpElicitationString(raw)
		if err != nil {
			return fmt.Errorf("MCP elicitation format must be a string: %w", err)
		}
		switch format {
		case "email", "uri", "date", "date-time":
		default:
			return fmt.Errorf("unknown MCP elicitation string format %q", format)
		}
	}
	return nil
}

func validateMcpElicitationNumberObject(object map[string]json.RawMessage) error {
	if err := allowOnlyMcpElicitationFields(object, "type", "title", "description", "minimum", "maximum", "default"); err != nil {
		return err
	}
	typeName, err := requiredMcpElicitationString(object, "type")
	if err != nil {
		return err
	}
	if typeName != "number" && typeName != "integer" {
		return fmt.Errorf("unknown MCP elicitation number type %q", typeName)
	}
	for _, name := range []string{"title", "description"} {
		if err := optionalMcpElicitationString(object, name); err != nil {
			return err
		}
	}
	for _, name := range []string{"minimum", "maximum", "default"} {
		if err := optionalMcpElicitationNumber(object, name); err != nil {
			return err
		}
	}
	return nil
}

func validateMcpElicitationBooleanObject(object map[string]json.RawMessage) error {
	if err := allowOnlyMcpElicitationFields(object, "type", "title", "description", "default"); err != nil {
		return err
	}
	if err := requireMcpElicitationLiteral(object, "type", "boolean"); err != nil {
		return err
	}
	for _, name := range []string{"title", "description"} {
		if err := optionalMcpElicitationString(object, name); err != nil {
			return err
		}
	}
	if raw, ok := object["default"]; ok && !isMcpElicitationNull(raw) {
		var value bool
		if err := json.Unmarshal(raw, &value); err != nil {
			return errors.New("MCP elicitation boolean default must be a boolean")
		}
	}
	return nil
}

func validateMcpElicitationStringEnumObject(object map[string]json.RawMessage, allowLegacyNames bool) error {
	allowed := []string{"type", "title", "description", "enum", "default"}
	if allowLegacyNames {
		allowed = append(allowed, "enumNames")
	}
	if err := allowOnlyMcpElicitationFields(object, allowed...); err != nil {
		return err
	}
	if err := requireMcpElicitationLiteral(object, "type", "string"); err != nil {
		return err
	}
	for _, name := range []string{"title", "description", "default"} {
		if err := optionalMcpElicitationString(object, name); err != nil {
			return err
		}
	}
	if err := requiredMcpElicitationStringArray(object, "enum"); err != nil {
		return err
	}
	if allowLegacyNames {
		if err := optionalMcpElicitationStringArray(object, "enumNames"); err != nil {
			return err
		}
	}
	return nil
}

func validateMcpElicitationTitledSingleObject(object map[string]json.RawMessage) error {
	if err := allowOnlyMcpElicitationFields(object, "type", "title", "description", "oneOf", "default"); err != nil {
		return err
	}
	if err := requireMcpElicitationLiteral(object, "type", "string"); err != nil {
		return err
	}
	for _, name := range []string{"title", "description", "default"} {
		if err := optionalMcpElicitationString(object, name); err != nil {
			return err
		}
	}
	return requiredMcpElicitationConstOptions(object, "oneOf")
}

func validateMcpElicitationMultiSelectObject(object map[string]json.RawMessage) error {
	if err := allowOnlyMcpElicitationFields(object, "type", "title", "description", "minItems", "maxItems", "items", "default"); err != nil {
		return err
	}
	if err := requireMcpElicitationLiteral(object, "type", "array"); err != nil {
		return err
	}
	for _, name := range []string{"title", "description"} {
		if err := optionalMcpElicitationString(object, name); err != nil {
			return err
		}
	}
	for _, name := range []string{"minItems", "maxItems"} {
		if err := optionalMcpElicitationUnsignedInteger(object, name); err != nil {
			return err
		}
	}
	if err := optionalMcpElicitationStringArray(object, "default"); err != nil {
		return err
	}
	itemsRaw, ok := object["items"]
	if !ok || isMcpElicitationNull(itemsRaw) {
		return errors.New("MCP elicitation multi-select schema requires items")
	}
	items, err := decodeMcpElicitationObject(itemsRaw)
	if err != nil {
		return fmt.Errorf("decode MCP elicitation multi-select items: %w", err)
	}
	if anyOf, ok := items["anyOf"]; ok {
		if err := allowOnlyMcpElicitationFields(items, "anyOf"); err != nil {
			return err
		}
		return validateMcpElicitationConstOptions(anyOf)
	}
	if oneOf, ok := items["oneOf"]; ok {
		if err := allowOnlyMcpElicitationFields(items, "oneOf"); err != nil {
			return err
		}
		if err := validateMcpElicitationConstOptions(oneOf); err != nil {
			return err
		}
		delete(items, "oneOf")
		items["anyOf"] = oneOf
		canonicalItems, err := canonicalMcpElicitationObject(items)
		if err != nil {
			return err
		}
		object["items"] = canonicalItems
		return nil
	}
	if err := allowOnlyMcpElicitationFields(items, "type", "enum"); err != nil {
		return err
	}
	if err := requireMcpElicitationLiteral(items, "type", "string"); err != nil {
		return err
	}
	return requiredMcpElicitationStringArray(items, "enum")
}

func validateMcpServerElicitationParamsJSON(data []byte) error {
	object, err := decodeMcpElicitationObject(data)
	if err != nil {
		return fmt.Errorf("decode MCP elicitation request params: %w", err)
	}
	for _, name := range []string{"threadId", "serverName", "mode", "message"} {
		if _, err := requiredMcpElicitationString(object, name); err != nil {
			return err
		}
	}
	if raw, ok := object["turnId"]; !ok || (!isMcpElicitationNull(raw) && !isMcpElicitationString(raw)) {
		return errors.New("MCP elicitation request requires nullable turnId")
	}
	if _, ok := object["_meta"]; !ok {
		return errors.New("MCP elicitation request requires _meta")
	}
	mode, _ := requiredMcpElicitationString(object, "mode")
	common := []string{"threadId", "turnId", "serverName", "mode", "_meta", "message", "requestId", "itemId", "startedAtMs", "serverId", "schema", "metadata", "reason"}
	switch mode {
	case "form":
		if err := allowOnlyMcpElicitationFields(object, append(common, "requestedSchema")...); err != nil {
			return err
		}
		raw, ok := object["requestedSchema"]
		if !ok {
			return errors.New("form MCP elicitation request requires requestedSchema")
		}
		if err := validateMcpElicitationSchemaJSON(raw); err != nil {
			return err
		}
	case "openai/form":
		if err := allowOnlyMcpElicitationFields(object, append(common, "requestedSchema")...); err != nil {
			return err
		}
		if _, ok := object["requestedSchema"]; !ok {
			return errors.New("openai/form MCP elicitation request requires requestedSchema")
		}
	case "url":
		if err := allowOnlyMcpElicitationFields(object, append(common, "url", "elicitationId")...); err != nil {
			return err
		}
		for _, name := range []string{"url", "elicitationId"} {
			if _, err := requiredMcpElicitationString(object, name); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unknown MCP elicitation mode %q", mode)
	}
	for _, name := range []string{"requestId", "itemId", "serverId", "reason"} {
		if err := optionalMcpElicitationString(object, name); err != nil {
			return err
		}
	}
	if raw, ok := object["startedAtMs"]; ok && (isMcpElicitationNull(raw) || !isMcpElicitationInteger(raw)) {
		return errors.New("MCP elicitation startedAtMs must be an integer")
	}
	for _, name := range []string{"schema", "metadata"} {
		if raw, ok := object[name]; ok && !isMcpElicitationNull(raw) {
			var value map[string]any
			if err := json.Unmarshal(raw, &value); err != nil || value == nil {
				return fmt.Errorf("MCP elicitation %s must be an object or null", name)
			}
		}
	}
	return nil
}

func validateMcpServerElicitationResponseJSON(data []byte) error {
	object, err := decodeMcpElicitationObject(data, "action", "content", "_meta")
	if err != nil {
		return fmt.Errorf("decode MCP elicitation response: %w", err)
	}
	action, err := requiredMcpElicitationString(object, "action")
	if err != nil {
		return err
	}
	switch action {
	case "accept", "decline", "cancel":
	default:
		return fmt.Errorf("unknown MCP elicitation action %q", action)
	}
	if _, ok := object["content"]; !ok {
		return errors.New("MCP elicitation response requires content")
	}
	if _, ok := object["_meta"]; !ok {
		return errors.New("MCP elicitation response requires _meta")
	}
	return nil
}

func decodeMcpElicitationObject(data []byte, allowed ...string) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return nil, err
	}
	if object == nil {
		return nil, errors.New("value must be an object")
	}
	if len(allowed) > 0 {
		if err := allowOnlyMcpElicitationFields(object, allowed...); err != nil {
			return nil, err
		}
	}
	return object, nil
}

func allowOnlyMcpElicitationFields(object map[string]json.RawMessage, allowed ...string) error {
	known := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		known[name] = struct{}{}
	}
	for name := range object {
		if _, ok := known[name]; !ok {
			return fmt.Errorf("unknown MCP elicitation field %q", name)
		}
	}
	return nil
}

func requiredMcpElicitationString(object map[string]json.RawMessage, name string) (string, error) {
	raw, ok := object[name]
	if !ok || isMcpElicitationNull(raw) {
		return "", fmt.Errorf("MCP elicitation requires %s", name)
	}
	value, err := decodeMcpElicitationString(raw)
	if err != nil {
		return "", fmt.Errorf("MCP elicitation %s must be a string", name)
	}
	return value, nil
}

func optionalMcpElicitationString(object map[string]json.RawMessage, name string) error {
	raw, ok := object[name]
	if !ok || isMcpElicitationNull(raw) {
		return nil
	}
	if _, err := decodeMcpElicitationString(raw); err != nil {
		return fmt.Errorf("MCP elicitation %s must be a string or null", name)
	}
	return nil
}

func requireMcpElicitationLiteral(object map[string]json.RawMessage, name, want string) error {
	value, err := requiredMcpElicitationString(object, name)
	if err != nil {
		return err
	}
	if value != want {
		return fmt.Errorf("MCP elicitation %s = %q, want %q", name, value, want)
	}
	return nil
}

func optionalMcpElicitationNumber(object map[string]json.RawMessage, name string) error {
	raw, ok := object[name]
	if !ok || isMcpElicitationNull(raw) {
		return nil
	}
	var value float64
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("MCP elicitation %s must be a number or null", name)
	}
	return nil
}

func optionalMcpElicitationUnsignedInteger(object map[string]json.RawMessage, name string) error {
	raw, ok := object[name]
	if !ok || isMcpElicitationNull(raw) {
		return nil
	}
	var value uint64
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("MCP elicitation %s must be a non-negative integer or null", name)
	}
	return nil
}

func requiredMcpElicitationStringArray(object map[string]json.RawMessage, name string) error {
	raw, ok := object[name]
	if !ok || isMcpElicitationNull(raw) {
		return fmt.Errorf("MCP elicitation requires %s array", name)
	}
	return validateMcpElicitationStringArray(raw, name)
}

func optionalMcpElicitationStringArray(object map[string]json.RawMessage, name string) error {
	raw, ok := object[name]
	if !ok || isMcpElicitationNull(raw) {
		return nil
	}
	return validateMcpElicitationStringArray(raw, name)
}

func validateMcpElicitationStringArray(raw json.RawMessage, name string) error {
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil || values == nil {
		return fmt.Errorf("MCP elicitation %s must be a string array", name)
	}
	return nil
}

func requiredMcpElicitationConstOptions(object map[string]json.RawMessage, name string) error {
	raw, ok := object[name]
	if !ok || isMcpElicitationNull(raw) {
		return fmt.Errorf("MCP elicitation requires %s options", name)
	}
	return validateMcpElicitationConstOptions(raw)
}

func validateMcpElicitationConstOptions(raw json.RawMessage) error {
	var options []json.RawMessage
	if err := json.Unmarshal(raw, &options); err != nil || options == nil {
		return errors.New("MCP elicitation titled options must be an array")
	}
	for index, optionRaw := range options {
		option, err := decodeMcpElicitationObject(optionRaw, "const", "title")
		if err != nil {
			return fmt.Errorf("decode MCP elicitation titled option %d: %w", index, err)
		}
		for _, name := range []string{"const", "title"} {
			if _, err := requiredMcpElicitationString(option, name); err != nil {
				return err
			}
		}
	}
	return nil
}

func canonicalMcpElicitationObject(object map[string]json.RawMessage) (json.RawMessage, error) {
	data, err := json.Marshal(object)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}

func decodeMcpElicitationString(raw json.RawMessage) (string, error) {
	var value string
	err := json.Unmarshal(raw, &value)
	return value, err
}

func isMcpElicitationNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func isMcpElicitationString(raw json.RawMessage) bool {
	_, err := decodeMcpElicitationString(raw)
	return err == nil
}

func isMcpElicitationInteger(raw json.RawMessage) bool {
	var value int64
	return json.Unmarshal(raw, &value) == nil
}
