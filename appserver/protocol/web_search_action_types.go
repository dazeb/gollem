package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// WebSearchAction retains one validated public web-search action without
// binding it to Gollem runtime items or provider-specific search payloads.
type WebSearchAction struct {
	raw json.RawMessage
}

func (a WebSearchAction) MarshalJSON() ([]byte, error) {
	if len(a.raw) == 0 {
		return nil, errors.New("web-search action has no value")
	}
	return validateWebSearchActionJSON(a.raw)
}

func (a *WebSearchAction) UnmarshalJSON(data []byte) error {
	if a == nil {
		return errors.New("decode web-search action into nil receiver")
	}
	canonical, err := validateWebSearchActionJSON(data)
	if err != nil {
		return err
	}
	a.raw = canonical
	return nil
}

func validateWebSearchActionJSON(data []byte) (json.RawMessage, error) {
	payload, err := decodeExactThreadItemObject(
		data,
		"web-search action",
		"type",
		"query",
		"queries",
		"url",
		"pattern",
	)
	if err != nil {
		return nil, err
	}
	actionType, err := decodeRequiredThreadItemValue[string](payload, "web-search action", "type")
	if err != nil {
		return nil, err
	}
	switch actionType {
	case "search":
		if err := rejectThreadItemFields(payload, "search web-search action", "type", "query", "queries"); err != nil {
			return nil, err
		}
		query, err := decodeRequiredNullableThreadItemString(payload, "search web-search action", "query")
		if err != nil {
			return nil, err
		}
		queries, err := decodeRequiredNullableWebSearchQueries(payload, "search web-search action", "queries")
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type    string    `json:"type"`
			Query   *string   `json:"query"`
			Queries *[]string `json:"queries"`
		}{Type: actionType, Query: query, Queries: queries})
	case "openPage":
		if err := rejectThreadItemFields(payload, "openPage web-search action", "type", "url"); err != nil {
			return nil, err
		}
		url, err := decodeRequiredNullableThreadItemString(payload, "openPage web-search action", "url")
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type string  `json:"type"`
			URL  *string `json:"url"`
		}{Type: actionType, URL: url})
	case "findInPage":
		if err := rejectThreadItemFields(payload, "findInPage web-search action", "type", "url", "pattern"); err != nil {
			return nil, err
		}
		url, err := decodeRequiredNullableThreadItemString(payload, "findInPage web-search action", "url")
		if err != nil {
			return nil, err
		}
		pattern, err := decodeRequiredNullableThreadItemString(payload, "findInPage web-search action", "pattern")
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type    string  `json:"type"`
			URL     *string `json:"url"`
			Pattern *string `json:"pattern"`
		}{Type: actionType, URL: url, Pattern: pattern})
	case "other":
		if err := rejectThreadItemFields(payload, "other web-search action", "type"); err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type string `json:"type"`
		}{Type: actionType})
	default:
		return nil, fmt.Errorf("unknown web-search action type %q", actionType)
	}
}

func decodeRequiredNullableWebSearchQueries(payload map[string]json.RawMessage, objectName, fieldName string) (*[]string, error) {
	raw, ok := payload[fieldName]
	if !ok {
		return nil, fmt.Errorf("%s requires %s", objectName, fieldName)
	}
	if isJSONNull(raw) {
		return nil, nil
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
