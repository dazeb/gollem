package protocol

import (
	"encoding/json"
	"errors"
)

type ThreadListResponse struct {
	Data            []Thread `json:"data" jsonschema:"nonnullable=true"`
	NextCursor      *string  `json:"nextCursor"`
	BackwardsCursor *string  `json:"backwardsCursor"`
}

func (r ThreadListResponse) MarshalJSON() ([]byte, error) {
	if r.Data == nil {
		return nil, errors.New("thread-list response data cannot be null")
	}
	type wire ThreadListResponse
	return json.Marshal(wire(r))
}

func (r *ThreadListResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode thread-list response into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(
		data,
		"thread-list response",
		"data",
		"nextCursor",
		"backwardsCursor",
	)
	if err != nil {
		return err
	}
	threads, err := decodeRequiredThreadItemValue[[]Thread](payload, "thread-list response", "data")
	if err != nil {
		return err
	}
	nextCursor, err := decodeRequiredNullableThreadItemValue[string](payload, "thread-list response", "nextCursor")
	if err != nil {
		return err
	}
	backwardsCursor, err := decodeRequiredNullableThreadItemValue[string](payload, "thread-list response", "backwardsCursor")
	if err != nil {
		return err
	}
	*r = ThreadListResponse{
		Data:            threads,
		NextCursor:      nextCursor,
		BackwardsCursor: backwardsCursor,
	}
	return nil
}

type ThreadReadResponse struct {
	Thread Thread `json:"thread"`
}

func (r *ThreadReadResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode thread-read response into nil receiver")
	}
	thread, err := decodePublicThreadResponse(data, "thread-read response")
	if err != nil {
		return err
	}
	*r = ThreadReadResponse{Thread: thread}
	return nil
}

type ThreadMetadataUpdateResponse struct {
	Thread Thread `json:"thread"`
}

func (r *ThreadMetadataUpdateResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode thread-metadata-update response into nil receiver")
	}
	thread, err := decodePublicThreadResponse(data, "thread-metadata-update response")
	if err != nil {
		return err
	}
	*r = ThreadMetadataUpdateResponse{Thread: thread}
	return nil
}

type ThreadUnarchiveResponse struct {
	Thread Thread `json:"thread"`
}

func (r *ThreadUnarchiveResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode thread-unarchive response into nil receiver")
	}
	thread, err := decodePublicThreadResponse(data, "thread-unarchive response")
	if err != nil {
		return err
	}
	*r = ThreadUnarchiveResponse{Thread: thread}
	return nil
}

func decodePublicThreadResponse(data []byte, objectName string) (Thread, error) {
	payload, err := decodeExactThreadItemObject(data, objectName, "thread")
	if err != nil {
		return Thread{}, err
	}
	return decodeRequiredThreadItemValue[Thread](payload, objectName, "thread")
}

var (
	_ json.Marshaler   = ThreadListResponse{}
	_ json.Unmarshaler = (*ThreadListResponse)(nil)
	_ json.Unmarshaler = (*ThreadReadResponse)(nil)
	_ json.Unmarshaler = (*ThreadMetadataUpdateResponse)(nil)
	_ json.Unmarshaler = (*ThreadUnarchiveResponse)(nil)
)
