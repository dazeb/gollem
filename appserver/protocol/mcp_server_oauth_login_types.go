package protocol

import (
	"encoding/json"
	"errors"
)

// McpServerOauthLoginParams is exact standalone public OAuth-login request
// data. It does not launch a browser, own credentials, or authorize a server.
type McpServerOauthLoginParams struct {
	Name        string    `json:"name"`
	ThreadID    *string   `json:"threadId"`
	Scopes      *[]string `json:"scopes,omitempty"`
	TimeoutSecs *int64    `json:"timeoutSecs,omitempty"`
}

func (p McpServerOauthLoginParams) MarshalJSON() ([]byte, error) {
	scopes := p.Scopes
	if scopes != nil && *scopes == nil {
		empty := []string{}
		scopes = &empty
	}
	return json.Marshal(struct {
		Name        string    `json:"name"`
		ThreadID    *string   `json:"threadId"`
		Scopes      *[]string `json:"scopes,omitempty"`
		TimeoutSecs *int64    `json:"timeoutSecs,omitempty"`
	}{
		Name: p.Name, ThreadID: p.ThreadID, Scopes: scopes, TimeoutSecs: p.TimeoutSecs,
	})
}

func (p *McpServerOauthLoginParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode MCP server OAuth login params into nil receiver")
	}
	const objectName = "MCP server OAuth login params"
	payload, err := decodeRustSerdeObject(
		data, objectName, "name", "threadId", "scopes", "timeoutSecs",
	)
	if err != nil {
		return err
	}
	name, err := decodeRequiredThreadItemValue[string](payload, objectName, "name")
	if err != nil {
		return err
	}
	threadID, err := decodeOptionalNullableConfigValue[string](payload, objectName, "threadId")
	if err != nil {
		return err
	}
	scopes, err := decodeOptionalNullableMcpOauthScopes(payload, objectName, "scopes")
	if err != nil {
		return err
	}
	timeoutSecs, err := decodeOptionalNullableConfigValue[int64](payload, objectName, "timeoutSecs")
	if err != nil {
		return err
	}
	*p = McpServerOauthLoginParams{
		Name: name, ThreadID: threadID, Scopes: scopes, TimeoutSecs: timeoutSecs,
	}
	return nil
}

func decodeOptionalNullableMcpOauthScopes(
	payload map[string]json.RawMessage,
	objectName string,
	fieldName string,
) (*[]string, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	values, err := decodeRequiredThreadItemArray[string](payload, objectName, fieldName)
	if err != nil {
		return nil, err
	}
	return &values, nil
}

// McpServerOauthLoginResponse is exact standalone public OAuth authorization
// URL data. The URL remains opaque to Gollem's protocol package.
type McpServerOauthLoginResponse struct {
	AuthorizationURL string `json:"authorizationUrl"`
}

func (r McpServerOauthLoginResponse) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		AuthorizationURL string `json:"authorizationUrl"`
	}{AuthorizationURL: r.AuthorizationURL})
}

func (r *McpServerOauthLoginResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode MCP server OAuth login response into nil receiver")
	}
	const objectName = "MCP server OAuth login response"
	payload, err := decodeRustSerdeObject(data, objectName, "authorizationUrl")
	if err != nil {
		return err
	}
	authorizationURL, err := decodeRequiredThreadItemValue[string](payload, objectName, "authorizationUrl")
	if err != nil {
		return err
	}
	*r = McpServerOauthLoginResponse{AuthorizationURL: authorizationURL}
	return nil
}

// McpServerOauthLoginCompletedNotification is exact standalone public OAuth
// completion data. The corresponding notification remains producerless.
type McpServerOauthLoginCompletedNotification struct {
	Name     string  `json:"name"`
	ThreadID *string `json:"threadId"`
	Success  bool    `json:"success"`
	Error    *string `json:"error,omitempty"`
}

func (n McpServerOauthLoginCompletedNotification) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Name     string  `json:"name"`
		ThreadID *string `json:"threadId"`
		Success  bool    `json:"success"`
		Error    *string `json:"error,omitempty"`
	}{Name: n.Name, ThreadID: n.ThreadID, Success: n.Success, Error: n.Error})
}

func (n *McpServerOauthLoginCompletedNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode MCP server OAuth login completion into nil receiver")
	}
	const objectName = "MCP server OAuth login completion"
	payload, err := decodeRustSerdeObject(data, objectName, "name", "threadId", "success", "error")
	if err != nil {
		return err
	}
	name, err := decodeRequiredThreadItemValue[string](payload, objectName, "name")
	if err != nil {
		return err
	}
	threadID, err := decodeOptionalNullableConfigValue[string](payload, objectName, "threadId")
	if err != nil {
		return err
	}
	success, err := decodeRequiredThreadItemValue[bool](payload, objectName, "success")
	if err != nil {
		return err
	}
	errorMessage, err := decodeOptionalNullableConfigValue[string](payload, objectName, "error")
	if err != nil {
		return err
	}
	*n = McpServerOauthLoginCompletedNotification{
		Name: name, ThreadID: threadID, Success: success, Error: errorMessage,
	}
	return nil
}

func mcpServerOauthLoginSchemas() map[string]Schema {
	nullableString := Schema{"type": []any{"string", "null"}}
	return map[string]Schema{
		"McpServerOauthLoginParams": {
			"type": "object",
			"properties": Schema{
				"name": Schema{"type": "string"},
				"scopes": Schema{
					"type":  []any{"array", "null"},
					"items": Schema{"type": "string"},
				},
				"threadId": nullableString,
				"timeoutSecs": Schema{
					"type": []any{"integer", "null"}, "format": "int64",
				},
			},
			"required": []string{"name"},
		},
		"McpServerOauthLoginResponse": {
			"type": "object",
			"properties": Schema{
				"authorizationUrl": Schema{"type": "string"},
			},
			"required": []string{"authorizationUrl"},
		},
		"McpServerOauthLoginCompletedNotification": {
			"type": "object",
			"properties": Schema{
				"error":    nullableString,
				"name":     Schema{"type": "string"},
				"success":  Schema{"type": "boolean"},
				"threadId": nullableString,
			},
			"required": []string{"name", "success"},
		},
	}
}

var (
	_ json.Marshaler   = McpServerOauthLoginParams{}
	_ json.Unmarshaler = (*McpServerOauthLoginParams)(nil)
	_ json.Marshaler   = McpServerOauthLoginResponse{}
	_ json.Unmarshaler = (*McpServerOauthLoginResponse)(nil)
	_ json.Marshaler   = McpServerOauthLoginCompletedNotification{}
	_ json.Unmarshaler = (*McpServerOauthLoginCompletedNotification)(nil)
)
