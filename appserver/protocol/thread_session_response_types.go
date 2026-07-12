package protocol

import (
	"encoding/json"
	"errors"
)

var threadSessionResponseRequiredFields = []string{
	"thread", "model", "modelProvider", "serviceTier", "cwd",
	"instructionSources", "approvalPolicy", "approvalsReviewer", "sandbox",
	"reasoningEffort",
}

type threadSessionResponse struct {
	Thread             Thread                `json:"thread"`
	Model              string                `json:"model"`
	ModelProvider      string                `json:"modelProvider"`
	ServiceTier        *string               `json:"serviceTier"`
	CWD                AbsolutePathBuf       `json:"cwd"`
	InstructionSources []LegacyAppPathString `json:"instructionSources" jsonschema:"nonnullable=true"`
	ApprovalPolicy     AskForApproval        `json:"approvalPolicy"`
	ApprovalsReviewer  ApprovalsReviewer     `json:"approvalsReviewer"`
	Sandbox            SandboxPolicy         `json:"sandbox"`
	ReasoningEffort    *ReasoningEffort      `json:"reasoningEffort"`
}

type ThreadStartResponse threadSessionResponse
type ThreadResumeResponse threadSessionResponse
type ThreadForkResponse threadSessionResponse

func (r ThreadStartResponse) MarshalJSON() ([]byte, error) {
	return marshalThreadSessionResponse(threadSessionResponse(r))
}

func (r *ThreadStartResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode thread-start response into nil receiver")
	}
	value, err := decodeThreadSessionResponse(data, "thread-start response")
	if err != nil {
		return err
	}
	*r = ThreadStartResponse(value)
	return nil
}

func (r ThreadResumeResponse) MarshalJSON() ([]byte, error) {
	return marshalThreadSessionResponse(threadSessionResponse(r))
}

func (r *ThreadResumeResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode thread-resume response into nil receiver")
	}
	value, err := decodeThreadSessionResponse(data, "thread-resume response")
	if err != nil {
		return err
	}
	*r = ThreadResumeResponse(value)
	return nil
}

func (r ThreadForkResponse) MarshalJSON() ([]byte, error) {
	return marshalThreadSessionResponse(threadSessionResponse(r))
}

func (r *ThreadForkResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode thread-fork response into nil receiver")
	}
	value, err := decodeThreadSessionResponse(data, "thread-fork response")
	if err != nil {
		return err
	}
	*r = ThreadForkResponse(value)
	return nil
}

func marshalThreadSessionResponse(response threadSessionResponse) ([]byte, error) {
	if response.InstructionSources == nil {
		return nil, errors.New("thread session response instructionSources cannot be null")
	}
	type wire threadSessionResponse
	return json.Marshal(wire(response))
}

func decodeThreadSessionResponse(data []byte, objectName string) (threadSessionResponse, error) {
	payload, err := decodeExactThreadItemObject(data, objectName, threadSessionResponseRequiredFields...)
	if err != nil {
		return threadSessionResponse{}, err
	}
	thread, err := decodeRequiredThreadItemValue[Thread](payload, objectName, "thread")
	if err != nil {
		return threadSessionResponse{}, err
	}
	model, err := decodeRequiredThreadItemValue[string](payload, objectName, "model")
	if err != nil {
		return threadSessionResponse{}, err
	}
	modelProvider, err := decodeRequiredThreadItemValue[string](payload, objectName, "modelProvider")
	if err != nil {
		return threadSessionResponse{}, err
	}
	serviceTier, err := decodeRequiredNullableThreadItemValue[string](payload, objectName, "serviceTier")
	if err != nil {
		return threadSessionResponse{}, err
	}
	cwd, err := decodeRequiredThreadItemValue[AbsolutePathBuf](payload, objectName, "cwd")
	if err != nil {
		return threadSessionResponse{}, err
	}
	instructionSources, err := decodeRequiredThreadItemArray[LegacyAppPathString](payload, objectName, "instructionSources")
	if err != nil {
		return threadSessionResponse{}, err
	}
	approvalPolicy, err := decodeRequiredThreadItemValue[AskForApproval](payload, objectName, "approvalPolicy")
	if err != nil {
		return threadSessionResponse{}, err
	}
	approvalsReviewer, err := decodeRequiredThreadItemValue[ApprovalsReviewer](payload, objectName, "approvalsReviewer")
	if err != nil {
		return threadSessionResponse{}, err
	}
	sandbox, err := decodeRequiredThreadItemValue[SandboxPolicy](payload, objectName, "sandbox")
	if err != nil {
		return threadSessionResponse{}, err
	}
	reasoningEffort, err := decodeRequiredNullableThreadItemValue[ReasoningEffort](payload, objectName, "reasoningEffort")
	if err != nil {
		return threadSessionResponse{}, err
	}
	return threadSessionResponse{
		Thread: thread, Model: model, ModelProvider: modelProvider,
		ServiceTier: serviceTier, CWD: cwd, InstructionSources: instructionSources,
		ApprovalPolicy: approvalPolicy, ApprovalsReviewer: approvalsReviewer,
		Sandbox: sandbox, ReasoningEffort: reasoningEffort,
	}, nil
}

var (
	_ json.Marshaler   = ThreadStartResponse{}
	_ json.Unmarshaler = (*ThreadStartResponse)(nil)
	_ json.Marshaler   = ThreadResumeResponse{}
	_ json.Unmarshaler = (*ThreadResumeResponse)(nil)
	_ json.Marshaler   = ThreadForkResponse{}
	_ json.Unmarshaler = (*ThreadForkResponse)(nil)
)
