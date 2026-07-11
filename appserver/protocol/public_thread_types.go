package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Thread is the exact public v2 thread projection. It remains separate from
// Gollem's durable ThreadRecord and is not yet bound to thread methods.
type Thread struct {
	ID             string          `json:"id"`
	SessionID      string          `json:"sessionId"`
	ForkedFromID   *string         `json:"forkedFromId"`
	ParentThreadID *string         `json:"parentThreadId"`
	Preview        string          `json:"preview"`
	Ephemeral      bool            `json:"ephemeral"`
	ModelProvider  string          `json:"modelProvider"`
	CreatedAt      int64           `json:"createdAt"`
	UpdatedAt      int64           `json:"updatedAt"`
	RecencyAt      *int64          `json:"recencyAt"`
	Status         ThreadStatus    `json:"status"`
	Path           *string         `json:"path"`
	CWD            AbsolutePathBuf `json:"cwd"`
	CLIVersion     string          `json:"cliVersion"`
	Source         SessionSource   `json:"source"`
	ThreadSource   *ThreadSource   `json:"threadSource"`
	AgentNickname  *string         `json:"agentNickname"`
	AgentRole      *string         `json:"agentRole"`
	GitInfo        *GitInfo        `json:"gitInfo"`
	Name           *string         `json:"name"`
	Turns          []Turn          `json:"turns" jsonschema:"nonnullable=true"`
}

func (t Thread) MarshalJSON() ([]byte, error) {
	if t.Turns == nil {
		return nil, errors.New("thread turns cannot be null")
	}
	type wire Thread
	encoded, err := json.Marshal(wire(t))
	if err != nil {
		return nil, fmt.Errorf("encode thread: %w", err)
	}
	return encoded, nil
}

func (t *Thread) UnmarshalJSON(data []byte) error {
	if t == nil {
		return errors.New("decode thread into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(
		data,
		"thread",
		"id",
		"sessionId",
		"forkedFromId",
		"parentThreadId",
		"preview",
		"ephemeral",
		"modelProvider",
		"createdAt",
		"updatedAt",
		"recencyAt",
		"status",
		"path",
		"cwd",
		"cliVersion",
		"source",
		"threadSource",
		"agentNickname",
		"agentRole",
		"gitInfo",
		"name",
		"turns",
	)
	if err != nil {
		return err
	}
	id, err := decodeRequiredThreadItemValue[string](payload, "thread", "id")
	if err != nil {
		return err
	}
	sessionID, err := decodeRequiredThreadItemValue[string](payload, "thread", "sessionId")
	if err != nil {
		return err
	}
	forkedFromID, err := decodeRequiredNullableThreadItemValue[string](payload, "thread", "forkedFromId")
	if err != nil {
		return err
	}
	parentThreadID, err := decodeRequiredNullableThreadItemValue[string](payload, "thread", "parentThreadId")
	if err != nil {
		return err
	}
	preview, err := decodeRequiredThreadItemValue[string](payload, "thread", "preview")
	if err != nil {
		return err
	}
	ephemeral, err := decodeRequiredThreadItemValue[bool](payload, "thread", "ephemeral")
	if err != nil {
		return err
	}
	modelProvider, err := decodeRequiredThreadItemValue[string](payload, "thread", "modelProvider")
	if err != nil {
		return err
	}
	createdAt, err := decodeRequiredThreadItemValue[int64](payload, "thread", "createdAt")
	if err != nil {
		return err
	}
	updatedAt, err := decodeRequiredThreadItemValue[int64](payload, "thread", "updatedAt")
	if err != nil {
		return err
	}
	recencyAt, err := decodeRequiredNullableThreadItemValue[int64](payload, "thread", "recencyAt")
	if err != nil {
		return err
	}
	status, err := decodeRequiredThreadItemValue[ThreadStatus](payload, "thread", "status")
	if err != nil {
		return err
	}
	path, err := decodeRequiredNullableThreadItemValue[string](payload, "thread", "path")
	if err != nil {
		return err
	}
	cwd, err := decodeRequiredThreadItemValue[AbsolutePathBuf](payload, "thread", "cwd")
	if err != nil {
		return err
	}
	cliVersion, err := decodeRequiredThreadItemValue[string](payload, "thread", "cliVersion")
	if err != nil {
		return err
	}
	source, err := decodeRequiredThreadItemValue[SessionSource](payload, "thread", "source")
	if err != nil {
		return err
	}
	threadSource, err := decodeRequiredNullableThreadItemValue[ThreadSource](payload, "thread", "threadSource")
	if err != nil {
		return err
	}
	agentNickname, err := decodeRequiredNullableThreadItemValue[string](payload, "thread", "agentNickname")
	if err != nil {
		return err
	}
	agentRole, err := decodeRequiredNullableThreadItemValue[string](payload, "thread", "agentRole")
	if err != nil {
		return err
	}
	gitInfo, err := decodeRequiredNullableThreadItemValue[GitInfo](payload, "thread", "gitInfo")
	if err != nil {
		return err
	}
	name, err := decodeRequiredNullableThreadItemValue[string](payload, "thread", "name")
	if err != nil {
		return err
	}
	turns, err := decodeRequiredThreadItemArray[Turn](payload, "thread", "turns")
	if err != nil {
		return err
	}
	*t = Thread{
		ID:             id,
		SessionID:      sessionID,
		ForkedFromID:   forkedFromID,
		ParentThreadID: parentThreadID,
		Preview:        preview,
		Ephemeral:      ephemeral,
		ModelProvider:  modelProvider,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
		RecencyAt:      recencyAt,
		Status:         status,
		Path:           path,
		CWD:            cwd,
		CLIVersion:     cliVersion,
		Source:         source,
		ThreadSource:   threadSource,
		AgentNickname:  agentNickname,
		AgentRole:      agentRole,
		GitInfo:        gitInfo,
		Name:           name,
		Turns:          turns,
	}
	return nil
}
