package protocol

import (
	"errors"
)

type McpToolCallAppContext struct {
	ConnectorID string  `json:"connectorId"`
	LinkID      *string `json:"linkId"`
	ResourceURI *string `json:"resourceUri"`
	AppName     *string `json:"appName"`
	TemplateID  *string `json:"templateId"`
	ActionName  *string `json:"actionName"`
}

func (c *McpToolCallAppContext) UnmarshalJSON(data []byte) error {
	if c == nil {
		return errors.New("decode MCP tool-call app context into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(
		data,
		"MCP tool-call app context",
		"connectorId",
		"linkId",
		"resourceUri",
		"appName",
		"templateId",
		"actionName",
	)
	if err != nil {
		return err
	}
	connectorID, err := decodeRequiredThreadItemValue[string](payload, "MCP tool-call app context", "connectorId")
	if err != nil {
		return err
	}
	linkID, err := decodeRequiredNullableThreadItemString(payload, "MCP tool-call app context", "linkId")
	if err != nil {
		return err
	}
	resourceURI, err := decodeRequiredNullableThreadItemString(payload, "MCP tool-call app context", "resourceUri")
	if err != nil {
		return err
	}
	appName, err := decodeRequiredNullableThreadItemString(payload, "MCP tool-call app context", "appName")
	if err != nil {
		return err
	}
	templateID, err := decodeRequiredNullableThreadItemString(payload, "MCP tool-call app context", "templateId")
	if err != nil {
		return err
	}
	actionName, err := decodeRequiredNullableThreadItemString(payload, "MCP tool-call app context", "actionName")
	if err != nil {
		return err
	}
	*c = McpToolCallAppContext{
		ConnectorID: connectorID,
		LinkID:      linkID,
		ResourceURI: resourceURI,
		AppName:     appName,
		TemplateID:  templateID,
		ActionName:  actionName,
	}
	return nil
}
