package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Resource is one exact public MCP resource descriptor.
type Resource struct {
	Annotations *JsonValue   `json:"annotations,omitempty"`
	Description *string      `json:"description,omitempty"`
	MimeType    *string      `json:"mimeType,omitempty"`
	Name        string       `json:"name"`
	Size        *int64       `json:"size,omitempty"`
	Title       *string      `json:"title,omitempty"`
	URI         string       `json:"uri"`
	Icons       *[]JsonValue `json:"icons,omitempty"`
	Meta        *JsonValue   `json:"_meta,omitempty"`
}

func (r Resource) MarshalJSON() ([]byte, error) {
	if r.Icons != nil && *r.Icons == nil {
		icons := []JsonValue{}
		r.Icons = &icons
	}
	type wire Resource
	return json.Marshal(wire(r))
}

func (r *Resource) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode MCP resource into nil receiver")
	}
	const objectName = "MCP resource"
	payload, err := decodeRustSerdeObject(
		data, objectName,
		"annotations", "description", "mimeType", "name", "size", "title", "uri", "icons", "_meta",
	)
	if err != nil {
		return err
	}
	name, err := decodeRequiredThreadItemValue[string](payload, objectName, "name")
	if err != nil {
		return err
	}
	uri, err := decodeRequiredThreadItemValue[string](payload, objectName, "uri")
	if err != nil {
		return err
	}
	annotations, err := decodeOptionalNullableMcpStatusValue[JsonValue](payload, objectName, "annotations")
	if err != nil {
		return err
	}
	description, err := decodeOptionalNullableMcpStatusValue[string](payload, objectName, "description")
	if err != nil {
		return err
	}
	mimeType, err := decodeOptionalNullableMcpStatusValue[string](payload, objectName, "mimeType")
	if err != nil {
		return err
	}
	size, err := decodeOptionalNullableMcpStatusValue[int64](payload, objectName, "size")
	if err != nil {
		return err
	}
	title, err := decodeOptionalNullableMcpStatusValue[string](payload, objectName, "title")
	if err != nil {
		return err
	}
	icons, err := decodeOptionalNullableMcpStatusValue[[]JsonValue](payload, objectName, "icons")
	if err != nil {
		return err
	}
	meta, err := decodeOptionalNullableMcpStatusValue[JsonValue](payload, objectName, "_meta")
	if err != nil {
		return err
	}
	*r = Resource{
		Annotations: annotations, Description: description, MimeType: mimeType, Name: name,
		Size: size, Title: title, URI: uri, Icons: icons, Meta: meta,
	}
	return nil
}

// ResourceTemplate is one exact public MCP resource-template descriptor.
type ResourceTemplate struct {
	Annotations *JsonValue `json:"annotations,omitempty"`
	URITemplate string     `json:"uriTemplate"`
	Name        string     `json:"name"`
	Title       *string    `json:"title,omitempty"`
	Description *string    `json:"description,omitempty"`
	MimeType    *string    `json:"mimeType,omitempty"`
}

func (r *ResourceTemplate) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode MCP resource template into nil receiver")
	}
	const objectName = "MCP resource template"
	payload, err := decodeRustSerdeObject(
		data, objectName, "annotations", "uriTemplate", "name", "title", "description", "mimeType",
	)
	if err != nil {
		return err
	}
	uriTemplate, err := decodeRequiredThreadItemValue[string](payload, objectName, "uriTemplate")
	if err != nil {
		return err
	}
	name, err := decodeRequiredThreadItemValue[string](payload, objectName, "name")
	if err != nil {
		return err
	}
	annotations, err := decodeOptionalNullableMcpStatusValue[JsonValue](payload, objectName, "annotations")
	if err != nil {
		return err
	}
	title, err := decodeOptionalNullableMcpStatusValue[string](payload, objectName, "title")
	if err != nil {
		return err
	}
	description, err := decodeOptionalNullableMcpStatusValue[string](payload, objectName, "description")
	if err != nil {
		return err
	}
	mimeType, err := decodeOptionalNullableMcpStatusValue[string](payload, objectName, "mimeType")
	if err != nil {
		return err
	}
	*r = ResourceTemplate{
		Annotations: annotations, URITemplate: uriTemplate, Name: name,
		Title: title, Description: description, MimeType: mimeType,
	}
	return nil
}

// Tool is one exact public MCP tool definition.
type Tool struct {
	Name         string       `json:"name"`
	Title        *string      `json:"title,omitempty"`
	Description  *string      `json:"description,omitempty"`
	InputSchema  JsonValue    `json:"inputSchema"`
	OutputSchema *JsonValue   `json:"outputSchema,omitempty"`
	Annotations  *JsonValue   `json:"annotations,omitempty"`
	Icons        *[]JsonValue `json:"icons,omitempty"`
	Meta         *JsonValue   `json:"_meta,omitempty"`
}

func (t Tool) MarshalJSON() ([]byte, error) {
	if t.Icons != nil && *t.Icons == nil {
		icons := []JsonValue{}
		t.Icons = &icons
	}
	type wire Tool
	return json.Marshal(wire(t))
}

func (t *Tool) UnmarshalJSON(data []byte) error {
	if t == nil {
		return errors.New("decode MCP tool into nil receiver")
	}
	const objectName = "MCP tool"
	payload, err := decodeRustSerdeObject(
		data, objectName,
		"name", "title", "description", "inputSchema", "outputSchema", "annotations", "icons", "_meta",
	)
	if err != nil {
		return err
	}
	name, err := decodeRequiredThreadItemValue[string](payload, objectName, "name")
	if err != nil {
		return err
	}
	inputSchema, err := decodeRequiredMcpStatusJSONValue(payload, objectName, "inputSchema")
	if err != nil {
		return err
	}
	title, err := decodeOptionalNullableMcpStatusValue[string](payload, objectName, "title")
	if err != nil {
		return err
	}
	description, err := decodeOptionalNullableMcpStatusValue[string](payload, objectName, "description")
	if err != nil {
		return err
	}
	outputSchema, err := decodeOptionalNullableMcpStatusValue[JsonValue](payload, objectName, "outputSchema")
	if err != nil {
		return err
	}
	annotations, err := decodeOptionalNullableMcpStatusValue[JsonValue](payload, objectName, "annotations")
	if err != nil {
		return err
	}
	icons, err := decodeOptionalNullableMcpStatusValue[[]JsonValue](payload, objectName, "icons")
	if err != nil {
		return err
	}
	meta, err := decodeOptionalNullableMcpStatusValue[JsonValue](payload, objectName, "_meta")
	if err != nil {
		return err
	}
	*t = Tool{
		Name: name, Title: title, Description: description, InputSchema: inputSchema,
		OutputSchema: outputSchema, Annotations: annotations, Icons: icons, Meta: meta,
	}
	return nil
}

// McpServerStatus is the exact public MCP inventory projection. Gollem's live
// status response remains a separate runtime contract until an adapter exists.
type McpServerStatus struct {
	Name              string             `json:"name"`
	ServerInfo        *McpServerInfo     `json:"serverInfo"`
	Tools             map[string]Tool    `json:"tools" jsonschema:"nonnullable=true"`
	Resources         []Resource         `json:"resources" jsonschema:"nonnullable=true"`
	ResourceTemplates []ResourceTemplate `json:"resourceTemplates" jsonschema:"nonnullable=true"`
	AuthStatus        McpAuthStatus      `json:"authStatus"`
}

func (s McpServerStatus) MarshalJSON() ([]byte, error) {
	if s.Tools == nil {
		return nil, errors.New("MCP server status tools cannot be null")
	}
	if s.Resources == nil {
		return nil, errors.New("MCP server status resources cannot be null")
	}
	if s.ResourceTemplates == nil {
		return nil, errors.New("MCP server status resourceTemplates cannot be null")
	}
	type wire McpServerStatus
	return json.Marshal(wire(s))
}

func (s *McpServerStatus) UnmarshalJSON(data []byte) error {
	if s == nil {
		return errors.New("decode MCP server status into nil receiver")
	}
	const objectName = "MCP server status"
	payload, err := decodeRustSerdeObject(
		data, objectName, "name", "serverInfo", "tools", "resources", "resourceTemplates", "authStatus",
	)
	if err != nil {
		return err
	}
	name, err := decodeRequiredThreadItemValue[string](payload, objectName, "name")
	if err != nil {
		return err
	}
	serverInfo, err := decodeOptionalNullableMcpStatusValue[McpServerInfo](payload, objectName, "serverInfo")
	if err != nil {
		return err
	}
	tools, err := decodeRequiredThreadItemValue[map[string]Tool](payload, objectName, "tools")
	if err != nil {
		return err
	}
	resources, err := decodeRequiredThreadItemArray[Resource](payload, objectName, "resources")
	if err != nil {
		return err
	}
	resourceTemplates, err := decodeRequiredThreadItemArray[ResourceTemplate](payload, objectName, "resourceTemplates")
	if err != nil {
		return err
	}
	authStatus, err := decodeRequiredThreadItemValue[McpAuthStatus](payload, objectName, "authStatus")
	if err != nil {
		return err
	}
	*s = McpServerStatus{
		Name: name, ServerInfo: serverInfo, Tools: tools, Resources: resources,
		ResourceTemplates: resourceTemplates, AuthStatus: authStatus,
	}
	return nil
}

// ListMcpServerStatusResponse is one exact public MCP inventory page.
type ListMcpServerStatusResponse struct {
	Data       []McpServerStatus `json:"data" jsonschema:"nonnullable=true"`
	NextCursor *string           `json:"nextCursor"`
}

func (r ListMcpServerStatusResponse) MarshalJSON() ([]byte, error) {
	if r.Data == nil {
		return nil, errors.New("MCP server status-list response data cannot be null")
	}
	type wire ListMcpServerStatusResponse
	return json.Marshal(wire(r))
}

func (r *ListMcpServerStatusResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode MCP server status-list response into nil receiver")
	}
	const objectName = "MCP server status-list response"
	payload, err := decodeRustSerdeObject(data, objectName, "data", "nextCursor")
	if err != nil {
		return err
	}
	dataValues, err := decodeRequiredThreadItemArray[McpServerStatus](payload, objectName, "data")
	if err != nil {
		return err
	}
	nextCursor, err := decodeOptionalNullableMcpStatusValue[string](payload, objectName, "nextCursor")
	if err != nil {
		return err
	}
	*r = ListMcpServerStatusResponse{Data: dataValues, NextCursor: nextCursor}
	return nil
}

func decodeOptionalNullableMcpStatusValue[T any](
	payload map[string]json.RawMessage,
	objectName string,
	fieldName string,
) (*T, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	return &value, nil
}

func decodeRequiredMcpStatusJSONValue(
	payload map[string]json.RawMessage,
	objectName string,
	fieldName string,
) (JsonValue, error) {
	raw, ok := payload[fieldName]
	if !ok {
		return JsonValue{}, fmt.Errorf("%s requires %s", objectName, fieldName)
	}
	var value JsonValue
	if err := json.Unmarshal(raw, &value); err != nil {
		return JsonValue{}, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	return value, nil
}

var (
	_ json.Marshaler   = Resource{}
	_ json.Unmarshaler = (*Resource)(nil)
	_ json.Unmarshaler = (*ResourceTemplate)(nil)
	_ json.Marshaler   = Tool{}
	_ json.Unmarshaler = (*Tool)(nil)
	_ json.Marshaler   = McpServerStatus{}
	_ json.Unmarshaler = (*McpServerStatus)(nil)
	_ json.Marshaler   = ListMcpServerStatusResponse{}
	_ json.Unmarshaler = (*ListMcpServerStatusResponse)(nil)
)
