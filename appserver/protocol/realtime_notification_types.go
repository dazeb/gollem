package protocol

import (
	"encoding/json"
	"errors"
)

// RealtimeConversationVersion is the exact closed realtime protocol version.
type RealtimeConversationVersion string

const (
	RealtimeConversationVersionV1 RealtimeConversationVersion = "v1"
	RealtimeConversationVersionV2 RealtimeConversationVersion = "v2"
	RealtimeConversationVersionV3 RealtimeConversationVersion = "v3"
)

func (v RealtimeConversationVersion) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(v, "realtime conversation version", RealtimeConversationVersion.valid)
}

func (v *RealtimeConversationVersion) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, v, "realtime conversation version", RealtimeConversationVersion.valid)
}

func (v RealtimeConversationVersion) valid() bool {
	return v == RealtimeConversationVersionV1 || v == RealtimeConversationVersionV2 || v == RealtimeConversationVersionV3
}

// ThreadRealtimeAudioChunk is an experimental realtime audio payload.
type ThreadRealtimeAudioChunk struct {
	Data              string  `json:"data"`
	SampleRate        uint32  `json:"sampleRate"`
	NumChannels       uint16  `json:"numChannels"`
	SamplesPerChannel *uint32 `json:"samplesPerChannel,omitempty"`
	ItemID            *string `json:"itemId,omitempty"`
}

func (c ThreadRealtimeAudioChunk) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Data              string  `json:"data"`
		SampleRate        uint32  `json:"sampleRate"`
		NumChannels       uint16  `json:"numChannels"`
		SamplesPerChannel *uint32 `json:"samplesPerChannel"`
		ItemID            *string `json:"itemId"`
	}{
		Data: c.Data, SampleRate: c.SampleRate, NumChannels: c.NumChannels,
		SamplesPerChannel: c.SamplesPerChannel, ItemID: c.ItemID,
	})
}

func (c *ThreadRealtimeAudioChunk) UnmarshalJSON(data []byte) error {
	if c == nil {
		return errors.New("decode thread realtime audio chunk into nil receiver")
	}
	const objectName = "thread realtime audio chunk"
	payload, err := decodeRustSerdeObject(data, objectName, "data", "sampleRate", "numChannels", "samplesPerChannel", "itemId")
	if err != nil {
		return err
	}
	value, err := decodeRequiredThreadItemValue[string](payload, objectName, "data")
	if err != nil {
		return err
	}
	sampleRate, err := decodeRequiredThreadItemValue[uint32](payload, objectName, "sampleRate")
	if err != nil {
		return err
	}
	numChannels, err := decodeRequiredThreadItemValue[uint16](payload, objectName, "numChannels")
	if err != nil {
		return err
	}
	samplesPerChannel, err := decodeOptionalNullableConfigValue[uint32](payload, objectName, "samplesPerChannel")
	if err != nil {
		return err
	}
	itemID, err := decodeOptionalNullableConfigValue[string](payload, objectName, "itemId")
	if err != nil {
		return err
	}
	*c = ThreadRealtimeAudioChunk{
		Data: value, SampleRate: sampleRate, NumChannels: numChannels,
		SamplesPerChannel: samplesPerChannel, ItemID: itemID,
	}
	return nil
}

// ThreadRealtimeStartedNotification reports accepted realtime startup.
type ThreadRealtimeStartedNotification struct {
	ThreadID          string                      `json:"threadId"`
	RealtimeSessionID *string                     `json:"realtimeSessionId,omitempty"`
	Version           RealtimeConversationVersion `json:"version"`
}

func (n ThreadRealtimeStartedNotification) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ThreadID          string                      `json:"threadId"`
		RealtimeSessionID *string                     `json:"realtimeSessionId"`
		Version           RealtimeConversationVersion `json:"version"`
	}{ThreadID: n.ThreadID, RealtimeSessionID: n.RealtimeSessionID, Version: n.Version})
}

func (n *ThreadRealtimeStartedNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode thread realtime started notification into nil receiver")
	}
	const objectName = "thread realtime started notification"
	payload, err := decodeRustSerdeObject(data, objectName, "threadId", "realtimeSessionId", "version")
	if err != nil {
		return err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, objectName, "threadId")
	if err != nil {
		return err
	}
	realtimeSessionID, err := decodeOptionalNullableConfigValue[string](payload, objectName, "realtimeSessionId")
	if err != nil {
		return err
	}
	version, err := decodeRequiredThreadItemValue[RealtimeConversationVersion](payload, objectName, "version")
	if err != nil {
		return err
	}
	*n = ThreadRealtimeStartedNotification{ThreadID: threadID, RealtimeSessionID: realtimeSessionID, Version: version}
	return nil
}

// ThreadRealtimeItemAddedNotification reports one raw realtime item.
type ThreadRealtimeItemAddedNotification struct {
	ThreadID string    `json:"threadId"`
	Item     JsonValue `json:"item"`
}

func (n *ThreadRealtimeItemAddedNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode thread realtime item-added notification into nil receiver")
	}
	const objectName = "thread realtime item-added notification"
	payload, err := decodeRustSerdeObject(data, objectName, "threadId", "item")
	if err != nil {
		return err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, objectName, "threadId")
	if err != nil {
		return err
	}
	item, err := decodeRequiredThreadItemJSONValue(payload, objectName, "item")
	if err != nil {
		return err
	}
	*n = ThreadRealtimeItemAddedNotification{ThreadID: threadID, Item: item}
	return nil
}

// ThreadRealtimeTranscriptDeltaNotification reports a live transcript delta.
type ThreadRealtimeTranscriptDeltaNotification struct {
	ThreadID string `json:"threadId"`
	Role     string `json:"role"`
	Delta    string `json:"delta"`
}

func (n *ThreadRealtimeTranscriptDeltaNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode thread realtime transcript-delta notification into nil receiver")
	}
	const objectName = "thread realtime transcript-delta notification"
	payload, err := decodeRustSerdeObject(data, objectName, "threadId", "role", "delta")
	if err != nil {
		return err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, objectName, "threadId")
	if err != nil {
		return err
	}
	role, err := decodeRequiredThreadItemValue[string](payload, objectName, "role")
	if err != nil {
		return err
	}
	delta, err := decodeRequiredThreadItemValue[string](payload, objectName, "delta")
	if err != nil {
		return err
	}
	*n = ThreadRealtimeTranscriptDeltaNotification{ThreadID: threadID, Role: role, Delta: delta}
	return nil
}

// ThreadRealtimeTranscriptDoneNotification reports final transcript text.
type ThreadRealtimeTranscriptDoneNotification struct {
	ThreadID string `json:"threadId"`
	Role     string `json:"role"`
	Text     string `json:"text"`
}

func (n *ThreadRealtimeTranscriptDoneNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode thread realtime transcript-done notification into nil receiver")
	}
	const objectName = "thread realtime transcript-done notification"
	payload, err := decodeRustSerdeObject(data, objectName, "threadId", "role", "text")
	if err != nil {
		return err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, objectName, "threadId")
	if err != nil {
		return err
	}
	role, err := decodeRequiredThreadItemValue[string](payload, objectName, "role")
	if err != nil {
		return err
	}
	text, err := decodeRequiredThreadItemValue[string](payload, objectName, "text")
	if err != nil {
		return err
	}
	*n = ThreadRealtimeTranscriptDoneNotification{ThreadID: threadID, Role: role, Text: text}
	return nil
}

// ThreadRealtimeOutputAudioDeltaNotification reports streamed output audio.
type ThreadRealtimeOutputAudioDeltaNotification struct {
	ThreadID string                   `json:"threadId"`
	Audio    ThreadRealtimeAudioChunk `json:"audio"`
}

func (n *ThreadRealtimeOutputAudioDeltaNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode thread realtime output-audio notification into nil receiver")
	}
	const objectName = "thread realtime output-audio notification"
	payload, err := decodeRustSerdeObject(data, objectName, "threadId", "audio")
	if err != nil {
		return err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, objectName, "threadId")
	if err != nil {
		return err
	}
	audio, err := decodeRequiredThreadItemValue[ThreadRealtimeAudioChunk](payload, objectName, "audio")
	if err != nil {
		return err
	}
	*n = ThreadRealtimeOutputAudioDeltaNotification{ThreadID: threadID, Audio: audio}
	return nil
}

// ThreadRealtimeSdpNotification reports remote WebRTC SDP.
type ThreadRealtimeSdpNotification struct {
	ThreadID string `json:"threadId"`
	SDP      string `json:"sdp"`
}

func (n *ThreadRealtimeSdpNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode thread realtime SDP notification into nil receiver")
	}
	const objectName = "thread realtime SDP notification"
	payload, err := decodeRustSerdeObject(data, objectName, "threadId", "sdp")
	if err != nil {
		return err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, objectName, "threadId")
	if err != nil {
		return err
	}
	sdp, err := decodeRequiredThreadItemValue[string](payload, objectName, "sdp")
	if err != nil {
		return err
	}
	*n = ThreadRealtimeSdpNotification{ThreadID: threadID, SDP: sdp}
	return nil
}

// ThreadRealtimeErrorNotification reports one realtime error.
type ThreadRealtimeErrorNotification struct {
	ThreadID string `json:"threadId"`
	Message  string `json:"message"`
}

func (n *ThreadRealtimeErrorNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode thread realtime error notification into nil receiver")
	}
	const objectName = "thread realtime error notification"
	payload, err := decodeRustSerdeObject(data, objectName, "threadId", "message")
	if err != nil {
		return err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, objectName, "threadId")
	if err != nil {
		return err
	}
	message, err := decodeRequiredThreadItemValue[string](payload, objectName, "message")
	if err != nil {
		return err
	}
	*n = ThreadRealtimeErrorNotification{ThreadID: threadID, Message: message}
	return nil
}

// ThreadRealtimeClosedNotification reports realtime transport closure.
type ThreadRealtimeClosedNotification struct {
	ThreadID string  `json:"threadId"`
	Reason   *string `json:"reason,omitempty"`
}

func (n ThreadRealtimeClosedNotification) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ThreadID string  `json:"threadId"`
		Reason   *string `json:"reason"`
	}{ThreadID: n.ThreadID, Reason: n.Reason})
}

func (n *ThreadRealtimeClosedNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode thread realtime closed notification into nil receiver")
	}
	const objectName = "thread realtime closed notification"
	payload, err := decodeRustSerdeObject(data, objectName, "threadId", "reason")
	if err != nil {
		return err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, objectName, "threadId")
	if err != nil {
		return err
	}
	reason, err := decodeOptionalNullableConfigValue[string](payload, objectName, "reason")
	if err != nil {
		return err
	}
	*n = ThreadRealtimeClosedNotification{ThreadID: threadID, Reason: reason}
	return nil
}

func realtimeNotificationSchemas() map[string]Schema {
	return map[string]Schema{
		"RealtimeConversationVersion": stringEnumSchema("v1", "v2", "v3"),
		"ThreadRealtimeAudioChunk": {
			"description": "EXPERIMENTAL - thread realtime audio chunk.",
			"properties": Schema{
				"data":              Schema{"type": "string"},
				"itemId":            Schema{"type": []any{"string", "null"}},
				"numChannels":       Schema{"format": "uint16", "minimum": float64(0), "type": "integer"},
				"sampleRate":        Schema{"format": "uint32", "minimum": float64(0), "type": "integer"},
				"samplesPerChannel": Schema{"format": "uint32", "minimum": float64(0), "type": []any{"integer", "null"}},
			},
			"required": []string{"data", "numChannels", "sampleRate"},
			"type":     "object",
		},
		"ThreadRealtimeClosedNotification": realtimeNotificationSchema(
			"EXPERIMENTAL - emitted when thread realtime transport closes.",
			"ThreadRealtimeClosedNotification",
			Schema{"reason": Schema{"type": []any{"string", "null"}}, "threadId": Schema{"type": "string"}},
			"threadId",
		),
		"ThreadRealtimeErrorNotification": realtimeNotificationSchema(
			"EXPERIMENTAL - emitted when thread realtime encounters an error.",
			"ThreadRealtimeErrorNotification",
			Schema{"message": Schema{"type": "string"}, "threadId": Schema{"type": "string"}},
			"message", "threadId",
		),
		"ThreadRealtimeItemAddedNotification": realtimeNotificationSchema(
			"EXPERIMENTAL - raw non-audio thread realtime item emitted by the backend.",
			"ThreadRealtimeItemAddedNotification",
			Schema{"item": true, "threadId": Schema{"type": "string"}},
			"item", "threadId",
		),
		"ThreadRealtimeOutputAudioDeltaNotification": realtimeNotificationSchema(
			"EXPERIMENTAL - streamed output audio emitted by thread realtime.",
			"ThreadRealtimeOutputAudioDeltaNotification",
			Schema{"audio": Schema{"$ref": "#/$defs/ThreadRealtimeAudioChunk"}, "threadId": Schema{"type": "string"}},
			"audio", "threadId",
		),
		"ThreadRealtimeSdpNotification": realtimeNotificationSchema(
			"EXPERIMENTAL - emitted with the remote SDP for a WebRTC realtime session.",
			"ThreadRealtimeSdpNotification",
			Schema{"sdp": Schema{"type": "string"}, "threadId": Schema{"type": "string"}},
			"sdp", "threadId",
		),
		"ThreadRealtimeStartedNotification": realtimeNotificationSchema(
			"EXPERIMENTAL - emitted when thread realtime startup is accepted.",
			"ThreadRealtimeStartedNotification",
			Schema{
				"realtimeSessionId": Schema{"type": []any{"string", "null"}},
				"threadId":          Schema{"type": "string"},
				"version":           Schema{"$ref": "#/$defs/RealtimeConversationVersion"},
			},
			"threadId", "version",
		),
		"ThreadRealtimeTranscriptDeltaNotification": realtimeNotificationSchema(
			"EXPERIMENTAL - flat transcript delta emitted whenever realtime transcript text changes.",
			"ThreadRealtimeTranscriptDeltaNotification",
			Schema{
				"delta":    Schema{"description": "Live transcript delta from the realtime event.", "type": "string"},
				"role":     Schema{"type": "string"},
				"threadId": Schema{"type": "string"},
			},
			"delta", "role", "threadId",
		),
		"ThreadRealtimeTranscriptDoneNotification": realtimeNotificationSchema(
			"EXPERIMENTAL - final transcript text emitted when realtime completes a transcript part.",
			"ThreadRealtimeTranscriptDoneNotification",
			Schema{
				"role":     Schema{"type": "string"},
				"text":     Schema{"description": "Final complete text for the transcript part.", "type": "string"},
				"threadId": Schema{"type": "string"},
			},
			"role", "text", "threadId",
		),
	}
}

func realtimeNotificationSchema(description, title string, properties Schema, required ...string) Schema {
	return Schema{
		"$schema":     "http://json-schema.org/draft-07/schema#",
		"description": description,
		"properties":  properties,
		"required":    required,
		"title":       title,
		"type":        "object",
	}
}
