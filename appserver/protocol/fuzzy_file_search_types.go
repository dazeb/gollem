package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// FuzzyFileSearchMatchType is the exact closed public kind of a fuzzy-search
// result. It does not imply that Gollem searched or accessed a path.
type FuzzyFileSearchMatchType string

const (
	FuzzyFileSearchMatchTypeFile      FuzzyFileSearchMatchType = "file"
	FuzzyFileSearchMatchTypeDirectory FuzzyFileSearchMatchType = "directory"
)

func (t FuzzyFileSearchMatchType) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(t, "fuzzy-file-search match type", FuzzyFileSearchMatchType.valid)
}

func (t *FuzzyFileSearchMatchType) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, t, "fuzzy-file-search match type", FuzzyFileSearchMatchType.valid)
}

func (t FuzzyFileSearchMatchType) valid() bool {
	switch t {
	case FuzzyFileSearchMatchTypeFile, FuzzyFileSearchMatchTypeDirectory:
		return true
	default:
		return false
	}
}

// FuzzyFileSearchParams is an exact standalone search description. Its roots
// remain opaque strings and grant no filesystem authority.
type FuzzyFileSearchParams struct {
	Query             string   `json:"query"`
	Roots             []string `json:"roots"`
	CancellationToken *string  `json:"cancellationToken"`
}

func (p FuzzyFileSearchParams) MarshalJSON() ([]byte, error) {
	roots := p.Roots
	if roots == nil {
		roots = []string{}
	}
	return json.Marshal(struct {
		Query             string   `json:"query"`
		Roots             []string `json:"roots"`
		CancellationToken *string  `json:"cancellationToken"`
	}{Query: p.Query, Roots: roots, CancellationToken: p.CancellationToken})
}

func (p *FuzzyFileSearchParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode fuzzy-file-search params into nil receiver")
	}
	const objectName = "fuzzy-file-search params"
	payload, err := decodeRustSerdeObject(
		data, objectName, "query", "roots", "cancellationToken",
	)
	if err != nil {
		return err
	}
	query, err := decodeRequiredThreadItemValue[string](payload, objectName, "query")
	if err != nil {
		return err
	}
	roots, err := decodeRequiredThreadItemArray[string](payload, objectName, "roots")
	if err != nil {
		return err
	}
	cancellationToken, err := decodeOptionalNullableFuzzyFileSearchValue[string](
		payload, objectName, "cancellationToken",
	)
	if err != nil {
		return err
	}
	*p = FuzzyFileSearchParams{
		Query: query, Roots: roots, CancellationToken: cancellationToken,
	}
	return nil
}

// FuzzyFileSearchResult is the exact standalone public result record. Scores,
// indices, and paths remain descriptive values with no ranking or path meaning.
type FuzzyFileSearchResult struct {
	Root      string                   `json:"root"`
	Path      string                   `json:"path"`
	MatchType FuzzyFileSearchMatchType `json:"match_type"`
	FileName  string                   `json:"file_name"`
	Score     uint32                   `json:"score"`
	Indices   *[]uint32                `json:"indices"`
}

func (r FuzzyFileSearchResult) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Root      string                   `json:"root"`
		Path      string                   `json:"path"`
		MatchType FuzzyFileSearchMatchType `json:"match_type"`
		FileName  string                   `json:"file_name"`
		Score     uint32                   `json:"score"`
		Indices   *[]uint32                `json:"indices"`
	}{
		Root: r.Root, Path: r.Path, MatchType: r.MatchType,
		FileName: r.FileName, Score: r.Score, Indices: r.Indices,
	})
}

func (r *FuzzyFileSearchResult) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode fuzzy-file-search result into nil receiver")
	}
	const objectName = "fuzzy-file-search result"
	payload, err := decodeRustSerdeObject(
		data, objectName, "root", "path", "match_type", "file_name", "score", "indices",
	)
	if err != nil {
		return err
	}
	root, err := decodeRequiredThreadItemValue[string](payload, objectName, "root")
	if err != nil {
		return err
	}
	path, err := decodeRequiredThreadItemValue[string](payload, objectName, "path")
	if err != nil {
		return err
	}
	matchType, err := decodeRequiredThreadItemValue[FuzzyFileSearchMatchType](
		payload, objectName, "match_type",
	)
	if err != nil {
		return err
	}
	fileName, err := decodeRequiredThreadItemValue[string](payload, objectName, "file_name")
	if err != nil {
		return err
	}
	score, err := decodeRequiredThreadItemValue[uint32](payload, objectName, "score")
	if err != nil {
		return err
	}
	indices, err := decodeOptionalNullableFuzzyFileSearchArray[uint32](
		payload, objectName, "indices",
	)
	if err != nil {
		return err
	}
	*r = FuzzyFileSearchResult{
		Root: root, Path: path, MatchType: matchType,
		FileName: fileName, Score: score, Indices: indices,
	}
	return nil
}

// FuzzyFileSearchResponse is an exact standalone fuzzy-search result envelope.
type FuzzyFileSearchResponse struct {
	Files []FuzzyFileSearchResult `json:"files"`
}

func (r FuzzyFileSearchResponse) MarshalJSON() ([]byte, error) {
	return marshalFuzzyFileSearchFiles(r.Files)
}

func (r *FuzzyFileSearchResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode fuzzy-file-search response into nil receiver")
	}
	files, err := unmarshalFuzzyFileSearchFiles(data, "fuzzy-file-search response")
	if err != nil {
		return err
	}
	*r = FuzzyFileSearchResponse{Files: files}
	return nil
}

// FuzzyFileSearchSessionUpdatedNotification is an exact standalone session
// snapshot. Gollem does not produce or bind this notification.
type FuzzyFileSearchSessionUpdatedNotification struct {
	SessionID string                  `json:"sessionId"`
	Query     string                  `json:"query"`
	Files     []FuzzyFileSearchResult `json:"files"`
}

func (n FuzzyFileSearchSessionUpdatedNotification) MarshalJSON() ([]byte, error) {
	files := n.Files
	if files == nil {
		files = []FuzzyFileSearchResult{}
	}
	return json.Marshal(struct {
		SessionID string                  `json:"sessionId"`
		Query     string                  `json:"query"`
		Files     []FuzzyFileSearchResult `json:"files"`
	}{SessionID: n.SessionID, Query: n.Query, Files: files})
}

func (n *FuzzyFileSearchSessionUpdatedNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode fuzzy-file-search session update into nil receiver")
	}
	const objectName = "fuzzy-file-search session update"
	payload, err := decodeRustSerdeObject(data, objectName, "sessionId", "query", "files")
	if err != nil {
		return err
	}
	sessionID, err := decodeRequiredThreadItemValue[string](payload, objectName, "sessionId")
	if err != nil {
		return err
	}
	query, err := decodeRequiredThreadItemValue[string](payload, objectName, "query")
	if err != nil {
		return err
	}
	files, err := decodeRequiredThreadItemArray[FuzzyFileSearchResult](payload, objectName, "files")
	if err != nil {
		return err
	}
	*n = FuzzyFileSearchSessionUpdatedNotification{
		SessionID: sessionID, Query: query, Files: files,
	}
	return nil
}

// FuzzyFileSearchSessionCompletedNotification is an exact standalone session
// completion marker. Gollem does not produce or bind this notification.
type FuzzyFileSearchSessionCompletedNotification struct {
	SessionID string `json:"sessionId"`
}

func (n FuzzyFileSearchSessionCompletedNotification) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		SessionID string `json:"sessionId"`
	}{SessionID: n.SessionID})
}

func (n *FuzzyFileSearchSessionCompletedNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode fuzzy-file-search session completion into nil receiver")
	}
	const objectName = "fuzzy-file-search session completion"
	payload, err := decodeRustSerdeObject(data, objectName, "sessionId")
	if err != nil {
		return err
	}
	sessionID, err := decodeRequiredThreadItemValue[string](payload, objectName, "sessionId")
	if err != nil {
		return err
	}
	*n = FuzzyFileSearchSessionCompletedNotification{SessionID: sessionID}
	return nil
}

func marshalFuzzyFileSearchFiles(files []FuzzyFileSearchResult) ([]byte, error) {
	if files == nil {
		files = []FuzzyFileSearchResult{}
	}
	return json.Marshal(struct {
		Files []FuzzyFileSearchResult `json:"files"`
	}{Files: files})
}

func unmarshalFuzzyFileSearchFiles(data []byte, objectName string) ([]FuzzyFileSearchResult, error) {
	payload, err := decodeRustSerdeObject(data, objectName, "files")
	if err != nil {
		return nil, err
	}
	return decodeRequiredThreadItemArray[FuzzyFileSearchResult](payload, objectName, "files")
}

func decodeOptionalNullableFuzzyFileSearchValue[T any](
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

func decodeOptionalNullableFuzzyFileSearchArray[T any](
	payload map[string]json.RawMessage,
	objectName string,
	fieldName string,
) (*[]T, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var elements []json.RawMessage
	if err := json.Unmarshal(raw, &elements); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	values := make([]T, len(elements))
	for index, element := range elements {
		if isJSONNull(element) {
			return nil, fmt.Errorf("decode %s %s[%d]: value cannot be null", objectName, fieldName, index)
		}
		if err := json.Unmarshal(element, &values[index]); err != nil {
			return nil, fmt.Errorf("decode %s %s[%d]: %w", objectName, fieldName, index, err)
		}
	}
	return &values, nil
}

func fuzzyFileSearchParamsSchema() Schema {
	return closedThreadSessionParamSchema(Schema{
		"query": Schema{"type": "string"},
		"roots": Schema{"type": "array", "items": Schema{"type": "string"}},
		"cancellationToken": Schema{"anyOf": []any{
			Schema{"type": "string"}, Schema{"type": "null"},
		}},
	}, []string{"query", "roots"})
}

func fuzzyFileSearchResultSchema() Schema {
	uint32Schema := Schema{"type": "integer", "minimum": 0, "maximum": 4294967295}
	schema := closedThreadSessionParamSchema(Schema{
		"root":       Schema{"type": "string"},
		"path":       Schema{"type": "string"},
		"match_type": Schema{"$ref": "#/$defs/FuzzyFileSearchMatchType"},
		"file_name":  Schema{"type": "string"},
		"score":      uint32Schema,
		"indices": Schema{"anyOf": []any{
			Schema{"type": "array", "items": uint32Schema}, Schema{"type": "null"},
		}},
	}, []string{"root", "path", "match_type", "file_name", "score"})
	schema["description"] = "Superset of [`codex_file_search::FileMatch`]"
	return schema
}

func fuzzyFileSearchResponseSchema() Schema {
	return closedThreadSessionParamSchema(Schema{
		"files": Schema{
			"type": "array", "items": Schema{"$ref": "#/$defs/FuzzyFileSearchResult"},
		},
	}, []string{"files"})
}

func fuzzyFileSearchSessionUpdatedNotificationSchema() Schema {
	return closedThreadSessionParamSchema(Schema{
		"sessionId": Schema{"type": "string"},
		"query":     Schema{"type": "string"},
		"files": Schema{
			"type": "array", "items": Schema{"$ref": "#/$defs/FuzzyFileSearchResult"},
		},
	}, []string{"sessionId", "query", "files"})
}

func fuzzyFileSearchSessionCompletedNotificationSchema() Schema {
	return closedThreadSessionParamSchema(Schema{
		"sessionId": Schema{"type": "string"},
	}, []string{"sessionId"})
}

var (
	_ json.Marshaler   = FuzzyFileSearchMatchType("")
	_ json.Unmarshaler = (*FuzzyFileSearchMatchType)(nil)
	_ json.Marshaler   = FuzzyFileSearchParams{}
	_ json.Unmarshaler = (*FuzzyFileSearchParams)(nil)
	_ json.Marshaler   = FuzzyFileSearchResult{}
	_ json.Unmarshaler = (*FuzzyFileSearchResult)(nil)
	_ json.Marshaler   = FuzzyFileSearchResponse{}
	_ json.Unmarshaler = (*FuzzyFileSearchResponse)(nil)
	_ json.Marshaler   = FuzzyFileSearchSessionUpdatedNotification{}
	_ json.Unmarshaler = (*FuzzyFileSearchSessionUpdatedNotification)(nil)
	_ json.Marshaler   = FuzzyFileSearchSessionCompletedNotification{}
	_ json.Unmarshaler = (*FuzzyFileSearchSessionCompletedNotification)(nil)
)
