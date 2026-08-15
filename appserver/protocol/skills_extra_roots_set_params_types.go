package protocol

import (
	"encoding/json"
	"errors"
)

// SkillsExtraRootsSetParams is the exact standalone source contract for
// skills/extraRoots/set. It intentionally does not configure skill discovery.
type SkillsExtraRootsSetParams struct {
	ExtraRoots []AbsolutePathBuf `json:"extraRoots"`
}

func (p SkillsExtraRootsSetParams) MarshalJSON() ([]byte, error) {
	extraRoots := p.ExtraRoots
	if extraRoots == nil {
		extraRoots = []AbsolutePathBuf{}
	}
	return json.Marshal(struct {
		ExtraRoots []AbsolutePathBuf `json:"extraRoots"`
	}{ExtraRoots: extraRoots})
}

func (p *SkillsExtraRootsSetParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode skills extra roots set params into nil receiver")
	}
	const objectName = "skills extra roots set params"
	payload, err := decodeRustSerdeObject(data, objectName, "extraRoots")
	if err != nil {
		return err
	}
	extraRoots, err := decodeRequiredThreadItemArray[AbsolutePathBuf](payload, objectName, "extraRoots")
	if err != nil {
		return err
	}
	*p = SkillsExtraRootsSetParams{ExtraRoots: extraRoots}
	return nil
}

func skillsExtraRootsSetParamSchema() Schema {
	return Schema{
		"properties": Schema{
			"extraRoots": Schema{
				"items": Schema{"$ref": "#/$defs/AbsolutePathBuf"},
				"type":  "array",
			},
		},
		"required": []string{"extraRoots"},
		"type":     "object",
	}
}

var (
	_ json.Marshaler   = SkillsExtraRootsSetParams{}
	_ json.Unmarshaler = (*SkillsExtraRootsSetParams)(nil)
)
