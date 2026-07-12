package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ThreadResumeInitialTurnsPageParams is the exact experimental initial-turn
// page selector. It remains standalone from the fixed thread/resume contract.
type ThreadResumeInitialTurnsPageParams struct {
	Limit         *uint32        `json:"limit,omitempty"`
	SortDirection *SortDirection `json:"sortDirection,omitempty"`
	ItemsView     *TurnItemsView `json:"itemsView,omitempty"`
}

func (p ThreadResumeInitialTurnsPageParams) MarshalJSON() ([]byte, error) {
	if p.SortDirection != nil && !validResumePageSortDirection(*p.SortDirection) {
		return nil, fmt.Errorf("invalid resume page sort direction %q", *p.SortDirection)
	}
	if p.ItemsView != nil && !p.ItemsView.valid() {
		return nil, fmt.Errorf("invalid resume page items view %q", *p.ItemsView)
	}
	type wire struct {
		Limit         *uint32        `json:"limit"`
		SortDirection *SortDirection `json:"sortDirection"`
		ItemsView     *TurnItemsView `json:"itemsView"`
	}
	return json.Marshal(wire(p))
}

func (p *ThreadResumeInitialTurnsPageParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode thread-resume initial-turn page params into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(
		data,
		"thread-resume initial-turn page params",
		"limit",
		"sortDirection",
		"itemsView",
	)
	if err != nil {
		return err
	}
	limit, err := decodeOptionalNullableResumePageValue[uint32](payload, "limit")
	if err != nil {
		return err
	}
	sortDirection, err := decodeOptionalNullableResumePageValue[SortDirection](payload, "sortDirection")
	if err != nil {
		return err
	}
	if sortDirection != nil && !validResumePageSortDirection(*sortDirection) {
		return fmt.Errorf("invalid resume page sort direction %q", *sortDirection)
	}
	itemsView, err := decodeOptionalNullableResumePageValue[TurnItemsView](payload, "itemsView")
	if err != nil {
		return err
	}
	*p = ThreadResumeInitialTurnsPageParams{
		Limit: limit, SortDirection: sortDirection, ItemsView: itemsView,
	}
	return nil
}

// TurnsPage is the exact public page of strict Turn projections. It remains
// standalone until Gollem implements the experimental resume pagination path.
type TurnsPage struct {
	Data            []Turn  `json:"data" jsonschema:"nonnullable=true"`
	NextCursor      *string `json:"nextCursor"`
	BackwardsCursor *string `json:"backwardsCursor"`
}

func (p TurnsPage) MarshalJSON() ([]byte, error) {
	if p.Data == nil {
		return nil, errors.New("turns page data cannot be null")
	}
	type wire TurnsPage
	encoded, err := json.Marshal(wire(p))
	if err != nil {
		return nil, fmt.Errorf("encode turns page: %w", err)
	}
	return encoded, nil
}

func (p *TurnsPage) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode turns page into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(
		data,
		"turns page",
		"data",
		"nextCursor",
		"backwardsCursor",
	)
	if err != nil {
		return err
	}
	turns, err := decodeRequiredThreadItemArray[Turn](payload, "turns page", "data")
	if err != nil {
		return err
	}
	nextCursor, err := decodeOptionalNullableResumePageValue[string](payload, "nextCursor")
	if err != nil {
		return err
	}
	backwardsCursor, err := decodeOptionalNullableResumePageValue[string](payload, "backwardsCursor")
	if err != nil {
		return err
	}
	*p = TurnsPage{Data: turns, NextCursor: nextCursor, BackwardsCursor: backwardsCursor}
	return nil
}

func decodeOptionalNullableResumePageValue[T any](
	payload map[string]json.RawMessage,
	fieldName string,
) (*T, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode resume page %s: %w", fieldName, err)
	}
	return &value, nil
}

func validResumePageSortDirection(direction SortDirection) bool {
	return direction == SortDirectionAsc || direction == SortDirectionDesc
}

var (
	_ json.Marshaler   = ThreadResumeInitialTurnsPageParams{}
	_ json.Unmarshaler = (*ThreadResumeInitialTurnsPageParams)(nil)
	_ json.Marshaler   = TurnsPage{}
	_ json.Unmarshaler = (*TurnsPage)(nil)
)
