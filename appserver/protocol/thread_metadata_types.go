package protocol

import (
	"encoding/json"
	"time"
)

type ThreadMemoryMode string

const (
	ThreadMemoryModeEnabled  ThreadMemoryMode = "enabled"
	ThreadMemoryModeDisabled ThreadMemoryMode = "disabled"
)

func (m ThreadMemoryMode) Valid() bool {
	return m == ThreadMemoryModeEnabled || m == ThreadMemoryModeDisabled
}

type ThreadMemoryModeSetParams struct {
	ThreadID   string           `json:"threadId,omitempty"`
	Mode       ThreadMemoryMode `json:"mode,omitempty"`
	ID         string           `json:"id,omitempty"`
	MemoryMode ThreadMemoryMode `json:"memoryMode,omitempty"`
}

func (p ThreadMemoryModeSetParams) EffectiveThreadID() string {
	return firstThreadControlID(p.ThreadID, p.ID)
}

func (p ThreadMemoryModeSetParams) EffectiveMode() ThreadMemoryMode {
	if p.Mode != "" {
		return p.Mode
	}
	return p.MemoryMode
}

type ThreadMemoryModeSetResponse struct {
	ThreadID   string           `json:"threadId,omitempty"`
	MemoryMode ThreadMemoryMode `json:"memoryMode,omitempty"`
	Thread     *ThreadRecord    `json:"thread,omitempty"`
}

type ThreadMetadataGitInfoUpdateParams struct {
	SHA       *string `json:"sha,omitempty"`
	Branch    *string `json:"branch,omitempty"`
	OriginURL *string `json:"originUrl,omitempty"`

	shaPresent       bool
	branchPresent    bool
	originURLPresent bool
}

func (p *ThreadMetadataGitInfoUpdateParams) UnmarshalJSON(data []byte) error {
	type wire ThreadMetadataGitInfoUpdateParams
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*p = ThreadMetadataGitInfoUpdateParams(decoded)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	_, p.shaPresent = fields["sha"]
	_, p.branchPresent = fields["branch"]
	_, p.originURLPresent = fields["originUrl"]
	return nil
}

func (p ThreadMetadataGitInfoUpdateParams) MarshalJSON() ([]byte, error) {
	type wire ThreadMetadataGitInfoUpdateParams
	nullFields := make([]string, 0, 3)
	if p.HasSHA() && p.SHA == nil {
		nullFields = append(nullFields, "sha")
	}
	if p.HasBranch() && p.Branch == nil {
		nullFields = append(nullFields, "branch")
	}
	if p.HasOriginURL() && p.OriginURL == nil {
		nullFields = append(nullFields, "originUrl")
	}
	return marshalWithExplicitNullFields(wire(p), nullFields)
}

func (p ThreadMetadataGitInfoUpdateParams) HasSHA() bool {
	return p.shaPresent || p.SHA != nil
}

func (p *ThreadMetadataGitInfoUpdateParams) SetSHA(value *string) {
	p.SHA = value
	p.shaPresent = true
}

func (p ThreadMetadataGitInfoUpdateParams) HasBranch() bool {
	return p.branchPresent || p.Branch != nil
}

func (p *ThreadMetadataGitInfoUpdateParams) SetBranch(value *string) {
	p.Branch = value
	p.branchPresent = true
}

func (p ThreadMetadataGitInfoUpdateParams) HasOriginURL() bool {
	return p.originURLPresent || p.OriginURL != nil
}

func (p *ThreadMetadataGitInfoUpdateParams) SetOriginURL(value *string) {
	p.OriginURL = value
	p.originURLPresent = true
}

func (p ThreadMetadataGitInfoUpdateParams) HasPatch() bool {
	return p.HasSHA() || p.HasBranch() || p.HasOriginURL()
}

type ThreadMetadataUpdateParams struct {
	ThreadID string                             `json:"threadId,omitempty"`
	GitInfo  *ThreadMetadataGitInfoUpdateParams `json:"gitInfo,omitempty"`
	ID       string                             `json:"id,omitempty"`
	Metadata map[string]any                     `json:"metadata,omitempty"`
	Replace  bool                               `json:"replace,omitempty"`

	gitInfoPresent bool
}

func (p *ThreadMetadataUpdateParams) UnmarshalJSON(data []byte) error {
	type wire ThreadMetadataUpdateParams
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*p = ThreadMetadataUpdateParams(decoded)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	_, p.gitInfoPresent = fields["gitInfo"]
	return nil
}

func (p ThreadMetadataUpdateParams) MarshalJSON() ([]byte, error) {
	type wire ThreadMetadataUpdateParams
	var nullFields []string
	if p.HasGitInfo() && p.GitInfo == nil {
		nullFields = []string{"gitInfo"}
	}
	return marshalWithExplicitNullFields(wire(p), nullFields)
}

func (p ThreadMetadataUpdateParams) EffectiveThreadID() string {
	return firstThreadControlID(p.ThreadID, p.ID)
}

func (p ThreadMetadataUpdateParams) HasGitInfo() bool {
	return p.gitInfoPresent || p.GitInfo != nil
}

func (p *ThreadMetadataUpdateParams) SetGitInfo(value *ThreadMetadataGitInfoUpdateParams) {
	p.GitInfo = value
	p.gitInfoPresent = true
}

type ThreadMetadataUpdateResult struct {
	Thread   ThreadRecord   `json:"thread"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type ThreadNameUpdatedNotification struct {
	ThreadID   string        `json:"threadId"`
	ThreadName *string       `json:"threadName,omitempty" jsonschema:"nonnullable=true"`
	Name       string        `json:"name,omitempty"`
	Thread     *ThreadRecord `json:"thread,omitempty"`
	At         *time.Time    `json:"at,omitempty"`
}

func marshalWithExplicitNullFields(value any, nullFields []string) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil || len(nullFields) == 0 {
		return data, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	for _, field := range nullFields {
		fields[field] = json.RawMessage("null")
	}
	return json.Marshal(fields)
}
