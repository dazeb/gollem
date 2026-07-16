package protocol

import (
	"encoding/json"
	"errors"
)

// AccountTokenUsageDailyBucket is exact standalone public account-usage data.
// Gollem does not fetch, aggregate, or persist account token usage.
type AccountTokenUsageDailyBucket struct {
	StartDate string `json:"startDate"`
	Tokens    int64  `json:"tokens"`
}

func (b AccountTokenUsageDailyBucket) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		StartDate string `json:"startDate"`
		Tokens    int64  `json:"tokens"`
	}{StartDate: b.StartDate, Tokens: b.Tokens})
}

func (b *AccountTokenUsageDailyBucket) UnmarshalJSON(data []byte) error {
	if b == nil {
		return errors.New("decode account token usage daily bucket into nil receiver")
	}
	const objectName = "account token usage daily bucket"
	payload, err := decodeRustSerdeObject(data, objectName, "startDate", "tokens")
	if err != nil {
		return err
	}
	startDate, err := decodeRequiredThreadItemValue[string](payload, objectName, "startDate")
	if err != nil {
		return err
	}
	tokens, err := decodeRequiredThreadItemValue[int64](payload, objectName, "tokens")
	if err != nil {
		return err
	}
	*b = AccountTokenUsageDailyBucket{StartDate: startDate, Tokens: tokens}
	return nil
}

// AccountTokenUsageSummary is exact standalone public account-usage summary
// data. Nullable values are canonicalized explicitly without aggregation.
type AccountTokenUsageSummary struct {
	LifetimeTokens        *int64 `json:"lifetimeTokens,omitempty"`
	PeakDailyTokens       *int64 `json:"peakDailyTokens,omitempty"`
	LongestRunningTurnSec *int64 `json:"longestRunningTurnSec,omitempty"`
	CurrentStreakDays     *int64 `json:"currentStreakDays,omitempty"`
	LongestStreakDays     *int64 `json:"longestStreakDays,omitempty"`
}

func (s AccountTokenUsageSummary) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		LifetimeTokens        *int64 `json:"lifetimeTokens"`
		PeakDailyTokens       *int64 `json:"peakDailyTokens"`
		LongestRunningTurnSec *int64 `json:"longestRunningTurnSec"`
		CurrentStreakDays     *int64 `json:"currentStreakDays"`
		LongestStreakDays     *int64 `json:"longestStreakDays"`
	}{
		LifetimeTokens: s.LifetimeTokens, PeakDailyTokens: s.PeakDailyTokens,
		LongestRunningTurnSec: s.LongestRunningTurnSec,
		CurrentStreakDays:     s.CurrentStreakDays, LongestStreakDays: s.LongestStreakDays,
	})
}

func (s *AccountTokenUsageSummary) UnmarshalJSON(data []byte) error {
	if s == nil {
		return errors.New("decode account token usage summary into nil receiver")
	}
	const objectName = "account token usage summary"
	payload, err := decodeRustSerdeObject(
		data, objectName,
		"lifetimeTokens", "peakDailyTokens", "longestRunningTurnSec",
		"currentStreakDays", "longestStreakDays",
	)
	if err != nil {
		return err
	}
	lifetimeTokens, err := decodeOptionalNullableConfigValue[int64](payload, objectName, "lifetimeTokens")
	if err != nil {
		return err
	}
	peakDailyTokens, err := decodeOptionalNullableConfigValue[int64](payload, objectName, "peakDailyTokens")
	if err != nil {
		return err
	}
	longestRunningTurnSec, err := decodeOptionalNullableConfigValue[int64](payload, objectName, "longestRunningTurnSec")
	if err != nil {
		return err
	}
	currentStreakDays, err := decodeOptionalNullableConfigValue[int64](payload, objectName, "currentStreakDays")
	if err != nil {
		return err
	}
	longestStreakDays, err := decodeOptionalNullableConfigValue[int64](payload, objectName, "longestStreakDays")
	if err != nil {
		return err
	}
	*s = AccountTokenUsageSummary{
		LifetimeTokens: lifetimeTokens, PeakDailyTokens: peakDailyTokens,
		LongestRunningTurnSec: longestRunningTurnSec,
		CurrentStreakDays:     currentStreakDays, LongestStreakDays: longestStreakDays,
	}
	return nil
}

var (
	_ json.Marshaler   = AccountTokenUsageDailyBucket{}
	_ json.Unmarshaler = (*AccountTokenUsageDailyBucket)(nil)
	_ json.Marshaler   = AccountTokenUsageSummary{}
	_ json.Unmarshaler = (*AccountTokenUsageSummary)(nil)
)
