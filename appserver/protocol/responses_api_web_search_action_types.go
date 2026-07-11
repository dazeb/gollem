package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ResponsesApiWebSearchAction is the provider-response action description. It
// remains distinct from the camel-case app-server WebSearchAction contract.
type ResponsesApiWebSearchAction struct {
	raw json.RawMessage
}

func (a ResponsesApiWebSearchAction) MarshalJSON() ([]byte, error) {
	if len(a.raw) == 0 {
		return nil, errors.New("responses API web-search action has no value")
	}
	return validateResponsesAPIWebSearchActionJSON(a.raw)
}

func (a *ResponsesApiWebSearchAction) UnmarshalJSON(data []byte) error {
	if a == nil {
		return errors.New("decode Responses API web-search action into nil receiver")
	}
	canonical, err := validateResponsesAPIWebSearchActionJSON(data)
	if err != nil {
		return err
	}
	a.raw = canonical
	return nil
}

func validateResponsesAPIWebSearchActionJSON(data []byte) (json.RawMessage, error) {
	payload, err := decodeExactThreadItemObject(
		data,
		"Responses API web-search action",
		"type",
		"query",
		"queries",
		"url",
		"pattern",
	)
	if err != nil {
		return nil, err
	}
	actionType, err := decodeRequiredThreadItemValue[string](payload, "Responses API web-search action", "type")
	if err != nil {
		return nil, err
	}
	switch actionType {
	case "search":
		if err := rejectThreadItemFields(payload, "Responses API search action", "type", "query", "queries"); err != nil {
			return nil, err
		}
		query, err := decodeOptionalNonNullResponsesAPIString(payload, "Responses API search action", "query")
		if err != nil {
			return nil, err
		}
		queries, err := decodeOptionalNonNullResponsesAPIQueries(payload, "Responses API search action", "queries")
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type    string    `json:"type"`
			Query   *string   `json:"query,omitempty"`
			Queries *[]string `json:"queries,omitempty"`
		}{Type: actionType, Query: query, Queries: queries})
	case "open_page":
		if err := rejectThreadItemFields(payload, "Responses API open-page action", "type", "url"); err != nil {
			return nil, err
		}
		url, err := decodeOptionalNonNullResponsesAPIString(payload, "Responses API open-page action", "url")
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type string  `json:"type"`
			URL  *string `json:"url,omitempty"`
		}{Type: actionType, URL: url})
	case "find_in_page":
		if err := rejectThreadItemFields(payload, "Responses API find-in-page action", "type", "url", "pattern"); err != nil {
			return nil, err
		}
		url, err := decodeOptionalNonNullResponsesAPIString(payload, "Responses API find-in-page action", "url")
		if err != nil {
			return nil, err
		}
		pattern, err := decodeOptionalNonNullResponsesAPIString(payload, "Responses API find-in-page action", "pattern")
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type    string  `json:"type"`
			URL     *string `json:"url,omitempty"`
			Pattern *string `json:"pattern,omitempty"`
		}{Type: actionType, URL: url, Pattern: pattern})
	case "other":
		if err := rejectThreadItemFields(payload, "Responses API other web-search action", "type"); err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type string `json:"type"`
		}{Type: actionType})
	default:
		return nil, fmt.Errorf("unknown Responses API web-search action type %q", actionType)
	}
}

func decodeOptionalNonNullResponsesAPIString(payload map[string]json.RawMessage, objectName, fieldName string) (*string, error) {
	raw, ok := payload[fieldName]
	if !ok {
		return nil, nil
	}
	if isJSONNull(raw) {
		return nil, fmt.Errorf("%s %s cannot be null", objectName, fieldName)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	return &value, nil
}

func decodeOptionalNonNullResponsesAPIQueries(payload map[string]json.RawMessage, objectName, fieldName string) (*[]string, error) {
	raw, ok := payload[fieldName]
	if !ok {
		return nil, nil
	}
	if isJSONNull(raw) {
		return nil, fmt.Errorf("%s %s cannot be null", objectName, fieldName)
	}
	var elements []json.RawMessage
	if err := json.Unmarshal(raw, &elements); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	values := make([]string, len(elements))
	for index, element := range elements {
		if isJSONNull(element) {
			return nil, fmt.Errorf("decode %s %s[%d]: expected string", objectName, fieldName, index)
		}
		if err := json.Unmarshal(element, &values[index]); err != nil {
			return nil, fmt.Errorf("decode %s %s[%d]: %w", objectName, fieldName, index, err)
		}
	}
	return &values, nil
}
