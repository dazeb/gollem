package protocol

import (
	"encoding/json"
	"errors"
)

type ToolRequestUserInputOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

func (o *ToolRequestUserInputOption) UnmarshalJSON(data []byte) error {
	if o == nil {
		return errors.New("decode tool request user input option into nil receiver")
	}
	const objectName = "tool request user input option"
	payload, err := decodeRustSerdeObject(data, objectName, "label", "description")
	if err != nil {
		return err
	}
	label, err := decodeRequiredThreadItemValue[string](payload, objectName, "label")
	if err != nil {
		return err
	}
	description, err := decodeRequiredThreadItemValue[string](payload, objectName, "description")
	if err != nil {
		return err
	}
	*o = ToolRequestUserInputOption{Label: label, Description: description}
	return nil
}

type ToolRequestUserInputQuestion struct {
	ID       string                       `json:"id"`
	Header   string                       `json:"header"`
	Question string                       `json:"question"`
	IsOther  bool                         `json:"isOther"`
	IsSecret bool                         `json:"isSecret"`
	Options  []ToolRequestUserInputOption `json:"options"`
}

func (q *ToolRequestUserInputQuestion) UnmarshalJSON(data []byte) error {
	if q == nil {
		return errors.New("decode tool request user input question into nil receiver")
	}
	const objectName = "tool request user input question"
	payload, err := decodeRustSerdeObject(data, objectName, "id", "header", "question", "isOther", "isSecret", "options")
	if err != nil {
		return err
	}
	id, err := decodeRequiredThreadItemValue[string](payload, objectName, "id")
	if err != nil {
		return err
	}
	header, err := decodeRequiredThreadItemValue[string](payload, objectName, "header")
	if err != nil {
		return err
	}
	question, err := decodeRequiredThreadItemValue[string](payload, objectName, "question")
	if err != nil {
		return err
	}
	isOther, err := decodeOptionalConfigBool(payload, objectName, "isOther")
	if err != nil {
		return err
	}
	isSecret, err := decodeOptionalConfigBool(payload, objectName, "isSecret")
	if err != nil {
		return err
	}
	options, err := decodeOptionalNullableConfigValue[[]ToolRequestUserInputOption](payload, objectName, "options")
	if err != nil {
		return err
	}
	*q = ToolRequestUserInputQuestion{
		ID:       id,
		Header:   header,
		Question: question,
		IsOther:  isOther,
		IsSecret: isSecret,
	}
	if options != nil {
		q.Options = *options
	}
	return nil
}

// ToolRequestUserInputParams is the public structured request.
type ToolRequestUserInputParams struct {
	ThreadID         string                         `json:"threadId"`
	TurnID           string                         `json:"turnId"`
	ItemID           string                         `json:"itemId"`
	Questions        []ToolRequestUserInputQuestion `json:"questions" jsonschema:"nonnullable=true"`
	IsBlocking       bool                           `json:"isBlocking"`
	AutoResolutionMS *uint64                        `json:"autoResolutionMs"`
}

func (p *ToolRequestUserInputParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode tool request user input params into nil receiver")
	}
	const objectName = "tool request user input params"
	payload, err := decodeRustSerdeObject(
		data,
		objectName,
		"threadId", "turnId", "itemId", "questions", "isBlocking", "autoResolutionMs",
	)
	if err != nil {
		return err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, objectName, "threadId")
	if err != nil {
		return err
	}
	turnID, err := decodeRequiredThreadItemValue[string](payload, objectName, "turnId")
	if err != nil {
		return err
	}
	itemID, err := decodeRequiredThreadItemValue[string](payload, objectName, "itemId")
	if err != nil {
		return err
	}
	questions, err := decodeRequiredThreadItemValue[[]ToolRequestUserInputQuestion](payload, objectName, "questions")
	if err != nil {
		return err
	}
	isBlocking, err := decodeOptionalNullableConfigValue[bool](payload, objectName, "isBlocking")
	if err != nil {
		return err
	}
	autoResolutionMS, err := decodeOptionalNullableConfigValue[uint64](payload, objectName, "autoResolutionMs")
	if err != nil {
		return err
	}
	blocking := true
	if isBlocking != nil {
		blocking = *isBlocking
	}
	*p = ToolRequestUserInputParams{
		ThreadID:         threadID,
		TurnID:           turnID,
		ItemID:           itemID,
		Questions:        questions,
		IsBlocking:       blocking,
		AutoResolutionMS: autoResolutionMS,
	}
	return nil
}

type ToolRequestUserInputAnswer struct {
	Answers []string `json:"answers" jsonschema:"nonnullable=true"`
}

func (a ToolRequestUserInputAnswer) MarshalJSON() ([]byte, error) {
	answers := a.Answers
	if answers == nil {
		answers = []string{}
	}
	return json.Marshal(struct {
		Answers []string `json:"answers"`
	}{Answers: answers})
}

func (a *ToolRequestUserInputAnswer) UnmarshalJSON(data []byte) error {
	if a == nil {
		return errors.New("decode user input answer into nil receiver")
	}
	const objectName = "tool request user input answer"
	payload, err := decodeRustSerdeObject(data, objectName, "answers")
	if err != nil {
		return err
	}
	answers, err := decodeRequiredThreadItemValue[[]string](payload, objectName, "answers")
	if err != nil {
		return err
	}
	*a = ToolRequestUserInputAnswer{Answers: answers}
	return nil
}

type ToolRequestUserInputResponse struct {
	Answers map[string]ToolRequestUserInputAnswer `json:"answers" jsonschema:"nonnullable=true"`
}

func (r ToolRequestUserInputResponse) MarshalJSON() ([]byte, error) {
	answers := r.Answers
	if answers == nil {
		answers = map[string]ToolRequestUserInputAnswer{}
	}
	return json.Marshal(struct {
		Answers map[string]ToolRequestUserInputAnswer `json:"answers"`
	}{Answers: answers})
}

func (r *ToolRequestUserInputResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode user input response into nil receiver")
	}
	const objectName = "tool request user input response"
	payload, err := decodeRustSerdeObject(data, objectName, "answers")
	if err != nil {
		return err
	}
	answers, err := decodeRequiredThreadItemValue[map[string]ToolRequestUserInputAnswer](payload, objectName, "answers")
	if err != nil {
		return err
	}
	*r = ToolRequestUserInputResponse{Answers: answers}
	return nil
}
