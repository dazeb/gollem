package protocol

import "errors"

// ThreadSearchResult is the exact public search result. It remains standalone
// because Gollem's live search response projects durable ThreadRecord values
// rather than the public Thread shape.
type ThreadSearchResult struct {
	Thread  Thread `json:"thread"`
	Snippet string `json:"snippet"`
}

func (r *ThreadSearchResult) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode thread-search result into nil receiver")
	}
	const objectName = "thread-search result"
	payload, err := decodeRustSerdeObject(data, objectName, "thread", "snippet")
	if err != nil {
		return err
	}
	thread, err := decodeRequiredThreadItemValue[Thread](payload, objectName, "thread")
	if err != nil {
		return err
	}
	snippet, err := decodeRequiredThreadItemValue[string](payload, objectName, "snippet")
	if err != nil {
		return err
	}
	*r = ThreadSearchResult{Thread: thread, Snippet: snippet}
	return nil
}

func threadSearchResultSchema() Schema {
	return Schema{
		"properties": Schema{
			"snippet": Schema{"type": "string"},
			"thread":  Schema{"$ref": "#/$defs/Thread"},
		},
		"required": []string{"snippet", "thread"},
		"type":     "object",
	}
}
