package appserver

import (
	"encoding/json"
	"strings"

	"github.com/fugue-labs/gollem/appserver/protocol"
)

const threadGitInfoMetadataKey = "gitInfo"

func normalizeThreadMetadataGitInfoPatch(params *protocol.ThreadMetadataGitInfoUpdateParams) (protocol.ThreadMetadataGitInfoUpdateParams, *protocol.Error) {
	if params == nil || !params.HasPatch() {
		return protocol.ThreadMetadataGitInfoUpdateParams{}, invalidParams("gitInfo must include at least one field", nil)
	}
	var normalized protocol.ThreadMetadataGitInfoUpdateParams
	for _, field := range []struct {
		name    string
		present bool
		value   *string
		set     func(*string)
	}{
		{name: "gitInfo.sha", present: params.HasSHA(), value: params.SHA, set: normalized.SetSHA},
		{name: "gitInfo.branch", present: params.HasBranch(), value: params.Branch, set: normalized.SetBranch},
		{name: "gitInfo.originUrl", present: params.HasOriginURL(), value: params.OriginURL, set: normalized.SetOriginURL},
	} {
		if !field.present {
			continue
		}
		if field.value == nil {
			field.set(nil)
			continue
		}
		value := strings.TrimSpace(*field.value)
		if value == "" {
			return protocol.ThreadMetadataGitInfoUpdateParams{}, invalidParams(field.name+" must not be empty", nil)
		}
		field.set(&value)
	}
	return normalized, nil
}

func applyThreadMetadataGitInfoPatch(current any, patch protocol.ThreadMetadataGitInfoUpdateParams) any {
	next := cloneMetadataObject(current)
	applyThreadMetadataGitField(next, "sha", patch.HasSHA(), patch.SHA)
	applyThreadMetadataGitField(next, "branch", patch.HasBranch(), patch.Branch)
	applyThreadMetadataGitField(next, "originUrl", patch.HasOriginURL(), patch.OriginURL)
	if len(next) == 0 {
		return nil
	}
	return next
}

func applyThreadMetadataGitField(metadata map[string]any, key string, present bool, value *string) {
	if !present {
		return
	}
	if value == nil {
		delete(metadata, key)
		return
	}
	metadata[key] = *value
}

func cloneMetadataObject(value any) map[string]any {
	object, ok := value.(map[string]any)
	if !ok && value != nil {
		if data, err := json.Marshal(value); err == nil {
			_ = json.Unmarshal(data, &object)
		}
	}
	if len(object) == 0 {
		return make(map[string]any)
	}
	cloned := make(map[string]any, len(object))
	for key, item := range object {
		cloned[key] = item
	}
	return cloned
}
