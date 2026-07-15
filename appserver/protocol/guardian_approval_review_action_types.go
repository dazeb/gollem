package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// GuardianApprovalReviewAction is the exact unstable public description of an
// action under approval review. It does not execute or authorize the action.
type GuardianApprovalReviewAction struct {
	raw json.RawMessage
}

func (a GuardianApprovalReviewAction) MarshalJSON() ([]byte, error) {
	if len(a.raw) == 0 {
		return nil, errors.New("guardian approval review action has no value")
	}
	return validateGuardianApprovalReviewActionJSON(a.raw)
}

func (a *GuardianApprovalReviewAction) UnmarshalJSON(data []byte) error {
	if a == nil {
		return errors.New("decode Guardian approval review action into nil receiver")
	}
	canonical, err := validateGuardianApprovalReviewActionJSON(data)
	if err != nil {
		return err
	}
	a.raw = canonical
	return nil
}

// Type returns the exact public action discriminant.
func (a GuardianApprovalReviewAction) Type() string {
	return permissionUnionDiscriminant(a.raw, "type")
}

func validateGuardianApprovalReviewActionJSON(data []byte) (json.RawMessage, error) {
	const objectName = "Guardian approval review action"
	payload, err := decodeRustSerdeObject(
		data,
		objectName,
		"type",
		"source",
		"command",
		"cwd",
		"program",
		"argv",
		"files",
		"target",
		"host",
		"protocol",
		"port",
		"server",
		"toolName",
		"connectorId",
		"connectorName",
		"toolTitle",
		"reason",
		"permissions",
	)
	if err != nil {
		return nil, err
	}
	actionType, err := decodeRequiredThreadItemValue[string](payload, objectName, "type")
	if err != nil {
		return nil, err
	}
	switch actionType {
	case "command":
		return canonicalGuardianCommandReviewAction(payload, actionType)
	case "execve":
		return canonicalGuardianExecveReviewAction(payload, actionType)
	case "applyPatch":
		return canonicalGuardianApplyPatchReviewAction(payload, actionType)
	case "networkAccess":
		return canonicalGuardianNetworkAccessReviewAction(payload, actionType)
	case "mcpToolCall":
		return canonicalGuardianMCPToolCallReviewAction(payload, actionType)
	case "requestPermissions":
		return canonicalGuardianRequestPermissionsReviewAction(payload, actionType)
	default:
		return nil, fmt.Errorf("unsupported Guardian approval review action type %q", actionType)
	}
}

func canonicalGuardianCommandReviewAction(
	payload map[string]json.RawMessage,
	actionType string,
) (json.RawMessage, error) {
	const objectName = "Guardian command review action"
	source, err := decodeRequiredThreadItemValue[GuardianCommandSource](payload, objectName, "source")
	if err != nil {
		return nil, err
	}
	command, err := decodeRequiredThreadItemValue[string](payload, objectName, "command")
	if err != nil {
		return nil, err
	}
	cwd, err := decodeRequiredThreadItemValue[AbsolutePathBuf](payload, objectName, "cwd")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type    string                `json:"type"`
		Source  GuardianCommandSource `json:"source"`
		Command string                `json:"command"`
		CWD     AbsolutePathBuf       `json:"cwd"`
	}{Type: actionType, Source: source, Command: command, CWD: cwd})
}

func canonicalGuardianExecveReviewAction(
	payload map[string]json.RawMessage,
	actionType string,
) (json.RawMessage, error) {
	const objectName = "Guardian execve review action"
	source, err := decodeRequiredThreadItemValue[GuardianCommandSource](payload, objectName, "source")
	if err != nil {
		return nil, err
	}
	program, err := decodeRequiredThreadItemValue[string](payload, objectName, "program")
	if err != nil {
		return nil, err
	}
	argv, err := decodeRequiredThreadItemArray[string](payload, objectName, "argv")
	if err != nil {
		return nil, err
	}
	cwd, err := decodeRequiredThreadItemValue[AbsolutePathBuf](payload, objectName, "cwd")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type    string                `json:"type"`
		Source  GuardianCommandSource `json:"source"`
		Program string                `json:"program"`
		Argv    []string              `json:"argv"`
		CWD     AbsolutePathBuf       `json:"cwd"`
	}{Type: actionType, Source: source, Program: program, Argv: argv, CWD: cwd})
}

func canonicalGuardianApplyPatchReviewAction(
	payload map[string]json.RawMessage,
	actionType string,
) (json.RawMessage, error) {
	const objectName = "Guardian apply-patch review action"
	cwd, err := decodeRequiredThreadItemValue[AbsolutePathBuf](payload, objectName, "cwd")
	if err != nil {
		return nil, err
	}
	files, err := decodeRequiredThreadItemArray[AbsolutePathBuf](payload, objectName, "files")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type  string            `json:"type"`
		CWD   AbsolutePathBuf   `json:"cwd"`
		Files []AbsolutePathBuf `json:"files"`
	}{Type: actionType, CWD: cwd, Files: files})
}

func canonicalGuardianNetworkAccessReviewAction(
	payload map[string]json.RawMessage,
	actionType string,
) (json.RawMessage, error) {
	const objectName = "Guardian network-access review action"
	target, err := decodeRequiredThreadItemValue[string](payload, objectName, "target")
	if err != nil {
		return nil, err
	}
	host, err := decodeRequiredThreadItemValue[string](payload, objectName, "host")
	if err != nil {
		return nil, err
	}
	protocol, err := decodeRequiredThreadItemValue[NetworkApprovalProtocol](payload, objectName, "protocol")
	if err != nil {
		return nil, err
	}
	port, err := decodeRequiredThreadItemValue[uint16](payload, objectName, "port")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type     string                  `json:"type"`
		Target   string                  `json:"target"`
		Host     string                  `json:"host"`
		Protocol NetworkApprovalProtocol `json:"protocol"`
		Port     uint16                  `json:"port"`
	}{Type: actionType, Target: target, Host: host, Protocol: protocol, Port: port})
}

func canonicalGuardianMCPToolCallReviewAction(
	payload map[string]json.RawMessage,
	actionType string,
) (json.RawMessage, error) {
	const objectName = "Guardian MCP tool-call review action"
	server, err := decodeRequiredThreadItemValue[string](payload, objectName, "server")
	if err != nil {
		return nil, err
	}
	toolName, err := decodeRequiredThreadItemValue[string](payload, objectName, "toolName")
	if err != nil {
		return nil, err
	}
	connectorID, err := decodeOptionalNullableConfigValue[string](payload, objectName, "connectorId")
	if err != nil {
		return nil, err
	}
	connectorName, err := decodeOptionalNullableConfigValue[string](payload, objectName, "connectorName")
	if err != nil {
		return nil, err
	}
	toolTitle, err := decodeOptionalNullableConfigValue[string](payload, objectName, "toolTitle")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type          string  `json:"type"`
		Server        string  `json:"server"`
		ToolName      string  `json:"toolName"`
		ConnectorID   *string `json:"connectorId"`
		ConnectorName *string `json:"connectorName"`
		ToolTitle     *string `json:"toolTitle"`
	}{
		Type: actionType, Server: server, ToolName: toolName,
		ConnectorID: connectorID, ConnectorName: connectorName, ToolTitle: toolTitle,
	})
}

func canonicalGuardianRequestPermissionsReviewAction(
	payload map[string]json.RawMessage,
	actionType string,
) (json.RawMessage, error) {
	const objectName = "Guardian request-permissions review action"
	reason, err := decodeOptionalNullableConfigValue[string](payload, objectName, "reason")
	if err != nil {
		return nil, err
	}
	permissions, err := decodeRequiredThreadItemValue[RequestPermissionProfile](
		payload, objectName, "permissions",
	)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type        string                   `json:"type"`
		Reason      *string                  `json:"reason"`
		Permissions RequestPermissionProfile `json:"permissions"`
	}{Type: actionType, Reason: reason, Permissions: permissions})
}

func guardianApprovalReviewActionSchema() Schema {
	nullableString := Schema{"type": []any{"string", "null"}}
	return Schema{"oneOf": []any{
		guardianApprovalReviewActionVariantSchema("command", []string{"source", "command", "cwd"}, Schema{
			"source":  Schema{"$ref": "#/$defs/GuardianCommandSource"},
			"command": Schema{"type": "string"},
			"cwd":     Schema{"$ref": "#/$defs/AbsolutePathBuf"},
		}),
		guardianApprovalReviewActionVariantSchema("execve", []string{"source", "program", "argv", "cwd"}, Schema{
			"source":  Schema{"$ref": "#/$defs/GuardianCommandSource"},
			"program": Schema{"type": "string"},
			"argv":    Schema{"type": "array", "items": Schema{"type": "string"}},
			"cwd":     Schema{"$ref": "#/$defs/AbsolutePathBuf"},
		}),
		guardianApprovalReviewActionVariantSchema("applyPatch", []string{"cwd", "files"}, Schema{
			"cwd":   Schema{"$ref": "#/$defs/AbsolutePathBuf"},
			"files": Schema{"type": "array", "items": Schema{"$ref": "#/$defs/AbsolutePathBuf"}},
		}),
		guardianApprovalReviewActionVariantSchema("networkAccess", []string{"target", "host", "protocol", "port"}, Schema{
			"target":   Schema{"type": "string"},
			"host":     Schema{"type": "string"},
			"protocol": Schema{"$ref": "#/$defs/NetworkApprovalProtocol"},
			"port":     Schema{"type": "integer", "minimum": 0, "maximum": 65535},
		}),
		guardianApprovalReviewActionVariantSchema("mcpToolCall", []string{"server", "toolName"}, Schema{
			"server":        Schema{"type": "string"},
			"toolName":      Schema{"type": "string"},
			"connectorId":   nullableString,
			"connectorName": nullableString,
			"toolTitle":     nullableString,
		}),
		guardianApprovalReviewActionVariantSchema("requestPermissions", []string{"permissions"}, Schema{
			"reason":      nullableString,
			"permissions": Schema{"$ref": "#/$defs/RequestPermissionProfile"},
		}),
	}}
}

func guardianApprovalReviewActionVariantSchema(
	actionType string,
	requiredFields []string,
	fields Schema,
) Schema {
	properties := Schema{"type": Schema{"type": "string", "enum": []any{actionType}}}
	for name, schema := range fields {
		properties[name] = schema
	}
	required := []string{"type"}
	required = append(required, requiredFields...)
	return Schema{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

var (
	_ json.Marshaler   = GuardianApprovalReviewAction{}
	_ json.Unmarshaler = (*GuardianApprovalReviewAction)(nil)
)
