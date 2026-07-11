package protocol

import (
	"encoding/json"
	"errors"
)

// WebSearchItem is the exact standalone public web-search record. It remains
// unbound from Gollem runtime items until the full public ThreadItem path exists.
type WebSearchItem struct {
	ID     string           `json:"id"`
	Query  string           `json:"query"`
	Action *WebSearchAction `json:"action"`
}

func (i WebSearchItem) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID     string           `json:"id"`
		Query  string           `json:"query"`
		Action *WebSearchAction `json:"action"`
	}{ID: i.ID, Query: i.Query, Action: i.Action})
}

func (i *WebSearchItem) UnmarshalJSON(data []byte) error {
	if i == nil {
		return errors.New("decode web-search item into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(data, "web-search item", "id", "query", "action")
	if err != nil {
		return err
	}
	id, err := decodeRequiredThreadItemValue[string](payload, "web-search item", "id")
	if err != nil {
		return err
	}
	query, err := decodeRequiredThreadItemValue[string](payload, "web-search item", "query")
	if err != nil {
		return err
	}
	action, err := decodeRequiredNullableThreadItemValue[WebSearchAction](payload, "web-search item", "action")
	if err != nil {
		return err
	}
	*i = WebSearchItem{ID: id, Query: query, Action: action}
	return nil
}
