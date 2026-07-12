package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

type ThreadStartParams struct {
	Model                 *string               `json:"model,omitempty"`
	ModelProvider         *string               `json:"modelProvider,omitempty"`
	ServiceTier           *string               `json:"serviceTier,omitempty"`
	CWD                   *string               `json:"cwd,omitempty"`
	ApprovalPolicy        *AskForApproval       `json:"approvalPolicy,omitempty"`
	ApprovalsReviewer     *ApprovalsReviewer    `json:"approvalsReviewer,omitempty"`
	Sandbox               *SandboxMode          `json:"sandbox,omitempty"`
	Config                *map[string]JsonValue `json:"config,omitempty"`
	ServiceName           *string               `json:"serviceName,omitempty"`
	BaseInstructions      *string               `json:"baseInstructions,omitempty"`
	DeveloperInstructions *string               `json:"developerInstructions,omitempty"`
	Personality           *Personality          `json:"personality,omitempty"`
	Ephemeral             *bool                 `json:"ephemeral,omitempty"`
	SessionStartSource    *ThreadStartSource    `json:"sessionStartSource,omitempty"`
	ThreadSource          *ThreadSource         `json:"threadSource,omitempty"`
}

type ThreadResumeParams struct {
	ThreadID              string                `json:"threadId"`
	Model                 *string               `json:"model,omitempty"`
	ModelProvider         *string               `json:"modelProvider,omitempty"`
	ServiceTier           *string               `json:"serviceTier,omitempty"`
	CWD                   *string               `json:"cwd,omitempty"`
	ApprovalPolicy        *AskForApproval       `json:"approvalPolicy,omitempty"`
	ApprovalsReviewer     *ApprovalsReviewer    `json:"approvalsReviewer,omitempty"`
	Sandbox               *SandboxMode          `json:"sandbox,omitempty"`
	Config                *map[string]JsonValue `json:"config,omitempty"`
	BaseInstructions      *string               `json:"baseInstructions,omitempty"`
	DeveloperInstructions *string               `json:"developerInstructions,omitempty"`
	Personality           *Personality          `json:"personality,omitempty"`
}

type ThreadForkParams struct {
	ThreadID              string                `json:"threadId"`
	LastTurnID            *string               `json:"lastTurnId,omitempty"`
	Model                 *string               `json:"model,omitempty"`
	ModelProvider         *string               `json:"modelProvider,omitempty"`
	ServiceTier           *string               `json:"serviceTier,omitempty"`
	CWD                   *string               `json:"cwd,omitempty"`
	ApprovalPolicy        *AskForApproval       `json:"approvalPolicy,omitempty"`
	ApprovalsReviewer     *ApprovalsReviewer    `json:"approvalsReviewer,omitempty"`
	Sandbox               *SandboxMode          `json:"sandbox,omitempty"`
	Config                *map[string]JsonValue `json:"config,omitempty"`
	BaseInstructions      *string               `json:"baseInstructions,omitempty"`
	DeveloperInstructions *string               `json:"developerInstructions,omitempty"`
	Ephemeral             *bool                 `json:"ephemeral,omitempty"`
	ThreadSource          *ThreadSource         `json:"threadSource,omitempty"`
}

type threadSessionParamOverrides struct {
	Model                 *string
	ModelProvider         *string
	ServiceTier           *string
	CWD                   *string
	ApprovalPolicy        *AskForApproval
	ApprovalsReviewer     *ApprovalsReviewer
	Sandbox               *SandboxMode
	Config                *map[string]JsonValue
	BaseInstructions      *string
	DeveloperInstructions *string
}

var threadSessionParamOverrideFields = []string{
	"model", "modelProvider", "serviceTier", "cwd", "approvalPolicy",
	"approvalsReviewer", "sandbox", "config", "baseInstructions", "developerInstructions",
}

func (p *ThreadStartParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode thread-start params into nil receiver")
	}
	allowed := append(append([]string{}, threadSessionParamOverrideFields...),
		"serviceName", "personality", "ephemeral", "sessionStartSource", "threadSource")
	payload, err := decodeExactThreadItemObject(data, "thread-start params", allowed...)
	if err != nil {
		return err
	}
	overrides, err := decodeThreadSessionParamOverrides(payload, "thread-start params")
	if err != nil {
		return err
	}
	serviceName, err := decodeOptionalNullableThreadSessionParam[string](payload, "thread-start params", "serviceName")
	if err != nil {
		return err
	}
	personality, err := decodeOptionalNullableThreadSessionParam[Personality](payload, "thread-start params", "personality")
	if err != nil {
		return err
	}
	ephemeral, err := decodeOptionalNullableThreadSessionParam[bool](payload, "thread-start params", "ephemeral")
	if err != nil {
		return err
	}
	sessionStartSource, err := decodeOptionalNullableThreadSessionParam[ThreadStartSource](payload, "thread-start params", "sessionStartSource")
	if err != nil {
		return err
	}
	threadSource, err := decodeOptionalNullableThreadSessionParam[ThreadSource](payload, "thread-start params", "threadSource")
	if err != nil {
		return err
	}
	*p = ThreadStartParams{
		Model: overrides.Model, ModelProvider: overrides.ModelProvider, ServiceTier: overrides.ServiceTier,
		CWD: overrides.CWD, ApprovalPolicy: overrides.ApprovalPolicy,
		ApprovalsReviewer: overrides.ApprovalsReviewer, Sandbox: overrides.Sandbox, Config: overrides.Config,
		ServiceName: serviceName, BaseInstructions: overrides.BaseInstructions,
		DeveloperInstructions: overrides.DeveloperInstructions, Personality: personality, Ephemeral: ephemeral,
		SessionStartSource: sessionStartSource, ThreadSource: threadSource,
	}
	return nil
}

func (p *ThreadResumeParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode thread-resume params into nil receiver")
	}
	allowed := append(append([]string{"threadId"}, threadSessionParamOverrideFields...), "personality")
	payload, err := decodeExactThreadItemObject(data, "thread-resume params", allowed...)
	if err != nil {
		return err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, "thread-resume params", "threadId")
	if err != nil {
		return err
	}
	overrides, err := decodeThreadSessionParamOverrides(payload, "thread-resume params")
	if err != nil {
		return err
	}
	personality, err := decodeOptionalNullableThreadSessionParam[Personality](payload, "thread-resume params", "personality")
	if err != nil {
		return err
	}
	*p = ThreadResumeParams{
		ThreadID: threadID, Model: overrides.Model, ModelProvider: overrides.ModelProvider,
		ServiceTier: overrides.ServiceTier, CWD: overrides.CWD, ApprovalPolicy: overrides.ApprovalPolicy,
		ApprovalsReviewer: overrides.ApprovalsReviewer, Sandbox: overrides.Sandbox, Config: overrides.Config,
		BaseInstructions: overrides.BaseInstructions, DeveloperInstructions: overrides.DeveloperInstructions,
		Personality: personality,
	}
	return nil
}

func (p *ThreadForkParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode thread-fork params into nil receiver")
	}
	allowed := append(append([]string{"threadId", "lastTurnId"}, threadSessionParamOverrideFields...),
		"ephemeral", "threadSource")
	payload, err := decodeExactThreadItemObject(data, "thread-fork params", allowed...)
	if err != nil {
		return err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, "thread-fork params", "threadId")
	if err != nil {
		return err
	}
	lastTurnID, err := decodeOptionalNullableThreadSessionParam[string](payload, "thread-fork params", "lastTurnId")
	if err != nil {
		return err
	}
	overrides, err := decodeThreadSessionParamOverrides(payload, "thread-fork params")
	if err != nil {
		return err
	}
	ephemeral, err := decodeOptionalNonNullThreadSessionParam[bool](payload, "thread-fork params", "ephemeral")
	if err != nil {
		return err
	}
	threadSource, err := decodeOptionalNullableThreadSessionParam[ThreadSource](payload, "thread-fork params", "threadSource")
	if err != nil {
		return err
	}
	*p = ThreadForkParams{
		ThreadID: threadID, LastTurnID: lastTurnID, Model: overrides.Model,
		ModelProvider: overrides.ModelProvider, ServiceTier: overrides.ServiceTier, CWD: overrides.CWD,
		ApprovalPolicy: overrides.ApprovalPolicy, ApprovalsReviewer: overrides.ApprovalsReviewer,
		Sandbox: overrides.Sandbox, Config: overrides.Config, BaseInstructions: overrides.BaseInstructions,
		DeveloperInstructions: overrides.DeveloperInstructions, Ephemeral: ephemeral, ThreadSource: threadSource,
	}
	return nil
}

func decodeThreadSessionParamOverrides(
	payload map[string]json.RawMessage,
	objectName string,
) (threadSessionParamOverrides, error) {
	model, err := decodeOptionalNullableThreadSessionParam[string](payload, objectName, "model")
	if err != nil {
		return threadSessionParamOverrides{}, err
	}
	modelProvider, err := decodeOptionalNullableThreadSessionParam[string](payload, objectName, "modelProvider")
	if err != nil {
		return threadSessionParamOverrides{}, err
	}
	serviceTier, err := decodeOptionalNullableThreadSessionParam[string](payload, objectName, "serviceTier")
	if err != nil {
		return threadSessionParamOverrides{}, err
	}
	cwd, err := decodeOptionalNullableThreadSessionParam[string](payload, objectName, "cwd")
	if err != nil {
		return threadSessionParamOverrides{}, err
	}
	approvalPolicy, err := decodeOptionalNullableThreadSessionParam[AskForApproval](payload, objectName, "approvalPolicy")
	if err != nil {
		return threadSessionParamOverrides{}, err
	}
	approvalsReviewer, err := decodeOptionalNullableThreadSessionParam[ApprovalsReviewer](payload, objectName, "approvalsReviewer")
	if err != nil {
		return threadSessionParamOverrides{}, err
	}
	sandbox, err := decodeOptionalNullableThreadSessionParam[SandboxMode](payload, objectName, "sandbox")
	if err != nil {
		return threadSessionParamOverrides{}, err
	}
	config, err := decodeOptionalNullableThreadSessionParam[map[string]JsonValue](payload, objectName, "config")
	if err != nil {
		return threadSessionParamOverrides{}, err
	}
	baseInstructions, err := decodeOptionalNullableThreadSessionParam[string](payload, objectName, "baseInstructions")
	if err != nil {
		return threadSessionParamOverrides{}, err
	}
	developerInstructions, err := decodeOptionalNullableThreadSessionParam[string](payload, objectName, "developerInstructions")
	if err != nil {
		return threadSessionParamOverrides{}, err
	}
	return threadSessionParamOverrides{
		Model: model, ModelProvider: modelProvider, ServiceTier: serviceTier, CWD: cwd,
		ApprovalPolicy: approvalPolicy, ApprovalsReviewer: approvalsReviewer, Sandbox: sandbox, Config: config,
		BaseInstructions: baseInstructions, DeveloperInstructions: developerInstructions,
	}, nil
}

func decodeOptionalNullableThreadSessionParam[T any](
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

func decodeOptionalNonNullThreadSessionParam[T any](
	payload map[string]json.RawMessage,
	objectName string,
	fieldName string,
) (*T, error) {
	raw, ok := payload[fieldName]
	if !ok {
		return nil, nil
	}
	if isJSONNull(raw) {
		return nil, fmt.Errorf("%s %s cannot be null", objectName, fieldName)
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	return &value, nil
}

var (
	_ json.Unmarshaler = (*ThreadStartParams)(nil)
	_ json.Unmarshaler = (*ThreadResumeParams)(nil)
	_ json.Unmarshaler = (*ThreadForkParams)(nil)
)
