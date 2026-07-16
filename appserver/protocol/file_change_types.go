package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// FileChange is the exact legacy public description of one file update. It is
// standalone and distinct from the live FileUpdateChange item payload.
type FileChange struct {
	raw json.RawMessage
}

func (c FileChange) MarshalJSON() ([]byte, error) {
	if len(c.raw) == 0 {
		return nil, errors.New("file change has no value")
	}
	return validateFileChangeJSON(c.raw)
}

func (c *FileChange) UnmarshalJSON(data []byte) error {
	if c == nil {
		return errors.New("decode file change into nil receiver")
	}
	canonical, err := validateFileChangeJSON(data)
	if err != nil {
		return err
	}
	c.raw = canonical
	return nil
}

// Type returns the exact public file-change discriminant.
func (c FileChange) Type() string {
	return permissionUnionDiscriminant(c.raw, "type")
}

func validateFileChangeJSON(data []byte) (json.RawMessage, error) {
	const objectName = "file change"
	discriminant, err := decodeRustSerdeObject(data, objectName, "type")
	if err != nil {
		return nil, err
	}
	changeType, err := decodeRequiredThreadItemValue[string](
		discriminant, objectName, "type",
	)
	if err != nil {
		return nil, err
	}
	switch changeType {
	case "add", "delete":
		return canonicalFileChangeContentVariant(data, changeType)
	case "update":
		return canonicalFileChangeUpdate(data)
	default:
		return nil, fmt.Errorf("unsupported file change type %q", changeType)
	}
}

func canonicalFileChangeContentVariant(data []byte, changeType string) (json.RawMessage, error) {
	const objectName = "file change"
	payload, err := decodeRustSerdeObject(data, objectName, "type", "content")
	if err != nil {
		return nil, err
	}
	content, err := decodeRequiredThreadItemValue[string](payload, objectName, "content")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type    string `json:"type"`
		Content string `json:"content"`
	}{Type: changeType, Content: content})
}

func canonicalFileChangeUpdate(data []byte) (json.RawMessage, error) {
	const objectName = "file change update"
	payload, err := decodeRustSerdeObject(
		data, objectName, "type", "unified_diff", "move_path",
	)
	if err != nil {
		return nil, err
	}
	unifiedDiff, err := decodeRequiredThreadItemValue[string](
		payload, objectName, "unified_diff",
	)
	if err != nil {
		return nil, err
	}
	movePath, err := decodeOptionalNullableConfigValue[string](
		payload, objectName, "move_path",
	)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type        string  `json:"type"`
		UnifiedDiff string  `json:"unified_diff"`
		MovePath    *string `json:"move_path"`
	}{Type: "update", UnifiedDiff: unifiedDiff, MovePath: movePath})
}

var (
	_ json.Marshaler   = FileChange{}
	_ json.Unmarshaler = (*FileChange)(nil)
)
