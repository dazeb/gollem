package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// PlanType is the public account plan classification. Unknown upstream strings
// collapse to the serde fallback literal instead of retaining provider data.
type PlanType string

const (
	PlanTypeFree                        PlanType = "free"
	PlanTypeGo                          PlanType = "go"
	PlanTypePlus                        PlanType = "plus"
	PlanTypePro                         PlanType = "pro"
	PlanTypeProLite                     PlanType = "prolite"
	PlanTypeTeam                        PlanType = "team"
	PlanTypeSelfServeBusinessUsageBased PlanType = "self_serve_business_usage_based"
	PlanTypeBusiness                    PlanType = "business"
	PlanTypeEnterpriseCBPUsageBased     PlanType = "enterprise_cbp_usage_based"
	PlanTypeEnterprise                  PlanType = "enterprise"
	PlanTypeEdu                         PlanType = "edu"
	PlanTypeUnknown                     PlanType = "unknown"
)

func (p PlanType) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(p, "plan type", PlanType.valid)
}

func (p *PlanType) UnmarshalJSON(data []byte) error {
	return unmarshalRateLimitFallbackEnum(data, p, "plan type", PlanType.valid, PlanTypeUnknown)
}

func (p PlanType) valid() bool {
	switch p {
	case PlanTypeFree, PlanTypeGo, PlanTypePlus, PlanTypePro, PlanTypeProLite,
		PlanTypeTeam, PlanTypeSelfServeBusinessUsageBased, PlanTypeBusiness,
		PlanTypeEnterpriseCBPUsageBased, PlanTypeEnterprise, PlanTypeEdu, PlanTypeUnknown:
		return true
	default:
		return false
	}
}

// RateLimitReachedType is the exact closed reason a public rate limit was hit.
type RateLimitReachedType string

const (
	RateLimitReachedTypeRateLimitReached                 RateLimitReachedType = "rate_limit_reached"
	RateLimitReachedTypeWorkspaceOwnerCreditsDepleted    RateLimitReachedType = "workspace_owner_credits_depleted"
	RateLimitReachedTypeWorkspaceMemberCreditsDepleted   RateLimitReachedType = "workspace_member_credits_depleted"
	RateLimitReachedTypeWorkspaceOwnerUsageLimitReached  RateLimitReachedType = "workspace_owner_usage_limit_reached"
	RateLimitReachedTypeWorkspaceMemberUsageLimitReached RateLimitReachedType = "workspace_member_usage_limit_reached"
)

func (r RateLimitReachedType) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(r, "rate-limit reached type", RateLimitReachedType.valid)
}

func (r *RateLimitReachedType) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, r, "rate-limit reached type", RateLimitReachedType.valid)
}

func (r RateLimitReachedType) valid() bool {
	switch r {
	case RateLimitReachedTypeRateLimitReached,
		RateLimitReachedTypeWorkspaceOwnerCreditsDepleted,
		RateLimitReachedTypeWorkspaceMemberCreditsDepleted,
		RateLimitReachedTypeWorkspaceOwnerUsageLimitReached,
		RateLimitReachedTypeWorkspaceMemberUsageLimitReached:
		return true
	default:
		return false
	}
}

// RateLimitResetType identifies the backend meter reset by a credit.
type RateLimitResetType string

const (
	RateLimitResetTypeCodexRateLimits RateLimitResetType = "codexRateLimits"
	RateLimitResetTypeUnknown         RateLimitResetType = "unknown"
)

func (r RateLimitResetType) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(r, "rate-limit reset type", RateLimitResetType.valid)
}

func (r *RateLimitResetType) UnmarshalJSON(data []byte) error {
	return unmarshalRateLimitFallbackEnum(
		data, r, "rate-limit reset type", RateLimitResetType.valid, RateLimitResetTypeUnknown,
	)
}

func (r RateLimitResetType) valid() bool {
	return r == RateLimitResetTypeCodexRateLimits || r == RateLimitResetTypeUnknown
}

// RateLimitResetCreditStatus is the public reset-credit lifecycle state.
type RateLimitResetCreditStatus string

const (
	RateLimitResetCreditStatusAvailable RateLimitResetCreditStatus = "available"
	RateLimitResetCreditStatusRedeeming RateLimitResetCreditStatus = "redeeming"
	RateLimitResetCreditStatusRedeemed  RateLimitResetCreditStatus = "redeemed"
	RateLimitResetCreditStatusUnknown   RateLimitResetCreditStatus = "unknown"
)

func (s RateLimitResetCreditStatus) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(s, "rate-limit reset credit status", RateLimitResetCreditStatus.valid)
}

func (s *RateLimitResetCreditStatus) UnmarshalJSON(data []byte) error {
	return unmarshalRateLimitFallbackEnum(
		data, s, "rate-limit reset credit status",
		RateLimitResetCreditStatus.valid, RateLimitResetCreditStatusUnknown,
	)
}

func (s RateLimitResetCreditStatus) valid() bool {
	switch s {
	case RateLimitResetCreditStatusAvailable, RateLimitResetCreditStatusRedeeming,
		RateLimitResetCreditStatusRedeemed, RateLimitResetCreditStatusUnknown:
		return true
	default:
		return false
	}
}

// ConsumeAccountRateLimitResetCreditOutcome is the exact closed redemption result.
type ConsumeAccountRateLimitResetCreditOutcome string

const (
	ConsumeAccountRateLimitResetCreditOutcomeReset           ConsumeAccountRateLimitResetCreditOutcome = "reset"
	ConsumeAccountRateLimitResetCreditOutcomeNothingToReset  ConsumeAccountRateLimitResetCreditOutcome = "nothingToReset"
	ConsumeAccountRateLimitResetCreditOutcomeNoCredit        ConsumeAccountRateLimitResetCreditOutcome = "noCredit"
	ConsumeAccountRateLimitResetCreditOutcomeAlreadyRedeemed ConsumeAccountRateLimitResetCreditOutcome = "alreadyRedeemed"
)

func (o ConsumeAccountRateLimitResetCreditOutcome) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(
		o, "consume account rate-limit reset credit outcome",
		ConsumeAccountRateLimitResetCreditOutcome.valid,
	)
}

func (o *ConsumeAccountRateLimitResetCreditOutcome) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(
		data, o, "consume account rate-limit reset credit outcome",
		ConsumeAccountRateLimitResetCreditOutcome.valid,
	)
}

func (o ConsumeAccountRateLimitResetCreditOutcome) valid() bool {
	switch o {
	case ConsumeAccountRateLimitResetCreditOutcomeReset,
		ConsumeAccountRateLimitResetCreditOutcomeNothingToReset,
		ConsumeAccountRateLimitResetCreditOutcomeNoCredit,
		ConsumeAccountRateLimitResetCreditOutcomeAlreadyRedeemed:
		return true
	default:
		return false
	}
}

func unmarshalRateLimitFallbackEnum[T ~string](
	data []byte,
	destination *T,
	name string,
	valid func(T) bool,
	unknown T,
) error {
	if destination == nil {
		return fmt.Errorf("decode %s into nil receiver", name)
	}
	if isJSONNull(data) {
		return fmt.Errorf("decode %s: value cannot be null", name)
	}
	var literal string
	if err := json.Unmarshal(data, &literal); err != nil {
		return fmt.Errorf("decode %s: %w", name, err)
	}
	value := T(literal)
	if !valid(value) {
		value = unknown
	}
	*destination = value
	return nil
}

// RateLimitWindow is one exact public usage window.
type RateLimitWindow struct {
	UsedPercent        int32  `json:"usedPercent"`
	WindowDurationMins *int64 `json:"windowDurationMins,omitempty"`
	ResetsAt           *int64 `json:"resetsAt,omitempty"`
}

func (w RateLimitWindow) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		UsedPercent        int32  `json:"usedPercent"`
		WindowDurationMins *int64 `json:"windowDurationMins"`
		ResetsAt           *int64 `json:"resetsAt"`
	}{w.UsedPercent, w.WindowDurationMins, w.ResetsAt})
}

func (w *RateLimitWindow) UnmarshalJSON(data []byte) error {
	if w == nil {
		return errors.New("decode rate-limit window into nil receiver")
	}
	const objectName = "rate-limit window"
	payload, err := decodeRustSerdeObject(data, objectName, "usedPercent", "windowDurationMins", "resetsAt")
	if err != nil {
		return err
	}
	usedPercent, err := decodeRequiredThreadItemValue[int32](payload, objectName, "usedPercent")
	if err != nil {
		return err
	}
	windowDurationMins, err := decodeOptionalNullableConfigValue[int64](payload, objectName, "windowDurationMins")
	if err != nil {
		return err
	}
	resetsAt, err := decodeOptionalNullableConfigValue[int64](payload, objectName, "resetsAt")
	if err != nil {
		return err
	}
	*w = RateLimitWindow{UsedPercent: usedPercent, WindowDurationMins: windowDurationMins, ResetsAt: resetsAt}
	return nil
}

// CreditsSnapshot is exact public account-credit availability data.
type CreditsSnapshot struct {
	HasCredits bool    `json:"hasCredits"`
	Unlimited  bool    `json:"unlimited"`
	Balance    *string `json:"balance,omitempty"`
}

func (s CreditsSnapshot) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		HasCredits bool    `json:"hasCredits"`
		Unlimited  bool    `json:"unlimited"`
		Balance    *string `json:"balance"`
	}{s.HasCredits, s.Unlimited, s.Balance})
}

func (s *CreditsSnapshot) UnmarshalJSON(data []byte) error {
	if s == nil {
		return errors.New("decode credits snapshot into nil receiver")
	}
	const objectName = "credits snapshot"
	payload, err := decodeRustSerdeObject(data, objectName, "hasCredits", "unlimited", "balance")
	if err != nil {
		return err
	}
	hasCredits, err := decodeRequiredThreadItemValue[bool](payload, objectName, "hasCredits")
	if err != nil {
		return err
	}
	unlimited, err := decodeRequiredThreadItemValue[bool](payload, objectName, "unlimited")
	if err != nil {
		return err
	}
	balance, err := decodeOptionalNullableConfigValue[string](payload, objectName, "balance")
	if err != nil {
		return err
	}
	*s = CreditsSnapshot{HasCredits: hasCredits, Unlimited: unlimited, Balance: balance}
	return nil
}

// SpendControlLimitSnapshot is exact public individual spend-control data.
type SpendControlLimitSnapshot struct {
	Limit            string `json:"limit"`
	Used             string `json:"used"`
	RemainingPercent int32  `json:"remainingPercent"`
	ResetsAt         int64  `json:"resetsAt"`
}

func (s SpendControlLimitSnapshot) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Limit            string `json:"limit"`
		Used             string `json:"used"`
		RemainingPercent int32  `json:"remainingPercent"`
		ResetsAt         int64  `json:"resetsAt"`
	}{s.Limit, s.Used, s.RemainingPercent, s.ResetsAt})
}

func (s *SpendControlLimitSnapshot) UnmarshalJSON(data []byte) error {
	if s == nil {
		return errors.New("decode spend-control limit snapshot into nil receiver")
	}
	const objectName = "spend-control limit snapshot"
	payload, err := decodeRustSerdeObject(data, objectName, "limit", "used", "remainingPercent", "resetsAt")
	if err != nil {
		return err
	}
	limit, err := decodeRequiredThreadItemValue[string](payload, objectName, "limit")
	if err != nil {
		return err
	}
	used, err := decodeRequiredThreadItemValue[string](payload, objectName, "used")
	if err != nil {
		return err
	}
	remainingPercent, err := decodeRequiredThreadItemValue[int32](payload, objectName, "remainingPercent")
	if err != nil {
		return err
	}
	resetsAt, err := decodeRequiredThreadItemValue[int64](payload, objectName, "resetsAt")
	if err != nil {
		return err
	}
	*s = SpendControlLimitSnapshot{Limit: limit, Used: used, RemainingPercent: remainingPercent, ResetsAt: resetsAt}
	return nil
}

// RateLimitSnapshot is a sparse standalone public rate-limit snapshot.
type RateLimitSnapshot struct {
	LimitID              *string                    `json:"limitId,omitempty"`
	LimitName            *string                    `json:"limitName,omitempty"`
	Primary              *RateLimitWindow           `json:"primary,omitempty"`
	Secondary            *RateLimitWindow           `json:"secondary,omitempty"`
	Credits              *CreditsSnapshot           `json:"credits,omitempty"`
	IndividualLimit      *SpendControlLimitSnapshot `json:"individualLimit,omitempty"`
	PlanType             *PlanType                  `json:"planType,omitempty"`
	RateLimitReachedType *RateLimitReachedType      `json:"rateLimitReachedType,omitempty"`
}

func (s RateLimitSnapshot) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		LimitID              *string                    `json:"limitId"`
		LimitName            *string                    `json:"limitName"`
		Primary              *RateLimitWindow           `json:"primary"`
		Secondary            *RateLimitWindow           `json:"secondary"`
		Credits              *CreditsSnapshot           `json:"credits"`
		IndividualLimit      *SpendControlLimitSnapshot `json:"individualLimit"`
		PlanType             *PlanType                  `json:"planType"`
		RateLimitReachedType *RateLimitReachedType      `json:"rateLimitReachedType"`
	}{s.LimitID, s.LimitName, s.Primary, s.Secondary, s.Credits, s.IndividualLimit, s.PlanType, s.RateLimitReachedType})
}

func (s *RateLimitSnapshot) UnmarshalJSON(data []byte) error {
	if s == nil {
		return errors.New("decode rate-limit snapshot into nil receiver")
	}
	const objectName = "rate-limit snapshot"
	payload, err := decodeRustSerdeObject(
		data, objectName, "limitId", "limitName", "primary", "secondary", "credits",
		"individualLimit", "planType", "rateLimitReachedType",
	)
	if err != nil {
		return err
	}
	limitID, err := decodeOptionalNullableConfigValue[string](payload, objectName, "limitId")
	if err != nil {
		return err
	}
	limitName, err := decodeOptionalNullableConfigValue[string](payload, objectName, "limitName")
	if err != nil {
		return err
	}
	primary, err := decodeOptionalNullableConfigValue[RateLimitWindow](payload, objectName, "primary")
	if err != nil {
		return err
	}
	secondary, err := decodeOptionalNullableConfigValue[RateLimitWindow](payload, objectName, "secondary")
	if err != nil {
		return err
	}
	credits, err := decodeOptionalNullableConfigValue[CreditsSnapshot](payload, objectName, "credits")
	if err != nil {
		return err
	}
	individualLimit, err := decodeOptionalNullableConfigValue[SpendControlLimitSnapshot](payload, objectName, "individualLimit")
	if err != nil {
		return err
	}
	planType, err := decodeOptionalNullableConfigValue[PlanType](payload, objectName, "planType")
	if err != nil {
		return err
	}
	rateLimitReachedType, err := decodeOptionalNullableConfigValue[RateLimitReachedType](payload, objectName, "rateLimitReachedType")
	if err != nil {
		return err
	}
	*s = RateLimitSnapshot{
		LimitID: limitID, LimitName: limitName, Primary: primary, Secondary: secondary,
		Credits: credits, IndividualLimit: individualLimit, PlanType: planType,
		RateLimitReachedType: rateLimitReachedType,
	}
	return nil
}

// RateLimitResetCredit is exact standalone reset-credit metadata.
type RateLimitResetCredit struct {
	ID          string                     `json:"id"`
	ResetType   RateLimitResetType         `json:"resetType"`
	Status      RateLimitResetCreditStatus `json:"status"`
	GrantedAt   int64                      `json:"grantedAt"`
	ExpiresAt   *int64                     `json:"expiresAt,omitempty"`
	Title       *string                    `json:"title,omitempty"`
	Description *string                    `json:"description,omitempty"`
}

func (c RateLimitResetCredit) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID          string                     `json:"id"`
		ResetType   RateLimitResetType         `json:"resetType"`
		Status      RateLimitResetCreditStatus `json:"status"`
		GrantedAt   int64                      `json:"grantedAt"`
		ExpiresAt   *int64                     `json:"expiresAt"`
		Title       *string                    `json:"title"`
		Description *string                    `json:"description"`
	}{c.ID, c.ResetType, c.Status, c.GrantedAt, c.ExpiresAt, c.Title, c.Description})
}

func (c *RateLimitResetCredit) UnmarshalJSON(data []byte) error {
	if c == nil {
		return errors.New("decode rate-limit reset credit into nil receiver")
	}
	const objectName = "rate-limit reset credit"
	payload, err := decodeRustSerdeObject(
		data, objectName, "id", "resetType", "status", "grantedAt", "expiresAt", "title", "description",
	)
	if err != nil {
		return err
	}
	id, err := decodeRequiredThreadItemValue[string](payload, objectName, "id")
	if err != nil {
		return err
	}
	resetType, err := decodeRequiredThreadItemValue[RateLimitResetType](payload, objectName, "resetType")
	if err != nil {
		return err
	}
	status, err := decodeRequiredThreadItemValue[RateLimitResetCreditStatus](payload, objectName, "status")
	if err != nil {
		return err
	}
	grantedAt, err := decodeRequiredThreadItemValue[int64](payload, objectName, "grantedAt")
	if err != nil {
		return err
	}
	expiresAt, err := decodeOptionalNullableConfigValue[int64](payload, objectName, "expiresAt")
	if err != nil {
		return err
	}
	title, err := decodeOptionalNullableConfigValue[string](payload, objectName, "title")
	if err != nil {
		return err
	}
	description, err := decodeOptionalNullableConfigValue[string](payload, objectName, "description")
	if err != nil {
		return err
	}
	*c = RateLimitResetCredit{
		ID: id, ResetType: resetType, Status: status, GrantedAt: grantedAt,
		ExpiresAt: expiresAt, Title: title, Description: description,
	}
	return nil
}

// RateLimitResetCreditsSummary is standalone reset-credit availability data.
type RateLimitResetCreditsSummary struct {
	AvailableCount int64                  `json:"availableCount"`
	Credits        []RateLimitResetCredit `json:"credits,omitempty"`
}

func (s RateLimitResetCreditsSummary) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		AvailableCount int64                  `json:"availableCount"`
		Credits        []RateLimitResetCredit `json:"credits"`
	}{s.AvailableCount, s.Credits})
}

func (s *RateLimitResetCreditsSummary) UnmarshalJSON(data []byte) error {
	if s == nil {
		return errors.New("decode rate-limit reset credits summary into nil receiver")
	}
	const objectName = "rate-limit reset credits summary"
	payload, err := decodeRustSerdeObject(data, objectName, "availableCount", "credits")
	if err != nil {
		return err
	}
	availableCount, err := decodeRequiredThreadItemValue[int64](payload, objectName, "availableCount")
	if err != nil {
		return err
	}
	credits, err := decodeOptionalNullableRateLimitValue[[]RateLimitResetCredit](payload, objectName, "credits")
	if err != nil {
		return err
	}
	*s = RateLimitResetCreditsSummary{AvailableCount: availableCount, Credits: credits}
	return nil
}

// GetAccountRateLimitsResponse is exact standalone rate-limit read data.
type GetAccountRateLimitsResponse struct {
	RateLimits            RateLimitSnapshot             `json:"rateLimits"`
	RateLimitsByLimitID   map[string]RateLimitSnapshot  `json:"rateLimitsByLimitId,omitempty"`
	RateLimitResetCredits *RateLimitResetCreditsSummary `json:"rateLimitResetCredits,omitempty"`
}

func (r GetAccountRateLimitsResponse) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		RateLimits            RateLimitSnapshot             `json:"rateLimits"`
		RateLimitsByLimitID   map[string]RateLimitSnapshot  `json:"rateLimitsByLimitId"`
		RateLimitResetCredits *RateLimitResetCreditsSummary `json:"rateLimitResetCredits"`
	}{r.RateLimits, r.RateLimitsByLimitID, r.RateLimitResetCredits})
}

func (r *GetAccountRateLimitsResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode get account rate limits response into nil receiver")
	}
	const objectName = "get account rate limits response"
	payload, err := decodeRustSerdeObject(
		data, objectName, "rateLimits", "rateLimitsByLimitId", "rateLimitResetCredits",
	)
	if err != nil {
		return err
	}
	rateLimits, err := decodeRequiredThreadItemValue[RateLimitSnapshot](payload, objectName, "rateLimits")
	if err != nil {
		return err
	}
	rateLimitsByLimitID, err := decodeOptionalNullableRateLimitValue[map[string]RateLimitSnapshot](
		payload, objectName, "rateLimitsByLimitId",
	)
	if err != nil {
		return err
	}
	rateLimitResetCredits, err := decodeOptionalNullableConfigValue[RateLimitResetCreditsSummary](
		payload, objectName, "rateLimitResetCredits",
	)
	if err != nil {
		return err
	}
	*r = GetAccountRateLimitsResponse{
		RateLimits: rateLimits, RateLimitsByLimitID: rateLimitsByLimitID,
		RateLimitResetCredits: rateLimitResetCredits,
	}
	return nil
}

// ConsumeAccountRateLimitResetCreditParams is a standalone redemption request.
// It does not perform or persist redemption.
type ConsumeAccountRateLimitResetCreditParams struct {
	IdempotencyKey string  `json:"idempotencyKey"`
	CreditID       *string `json:"creditId,omitempty"`
}

func (p ConsumeAccountRateLimitResetCreditParams) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		IdempotencyKey string  `json:"idempotencyKey"`
		CreditID       *string `json:"creditId"`
	}{p.IdempotencyKey, p.CreditID})
}

func (p *ConsumeAccountRateLimitResetCreditParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode consume account rate-limit reset credit params into nil receiver")
	}
	const objectName = "consume account rate-limit reset credit params"
	payload, err := decodeRustSerdeObject(data, objectName, "idempotencyKey", "creditId")
	if err != nil {
		return err
	}
	idempotencyKey, err := decodeRequiredThreadItemValue[string](payload, objectName, "idempotencyKey")
	if err != nil {
		return err
	}
	creditID, err := decodeOptionalNullableConfigValue[string](payload, objectName, "creditId")
	if err != nil {
		return err
	}
	*p = ConsumeAccountRateLimitResetCreditParams{IdempotencyKey: idempotencyKey, CreditID: creditID}
	return nil
}

// ConsumeAccountRateLimitResetCreditResponse is the standalone redemption outcome.
type ConsumeAccountRateLimitResetCreditResponse struct {
	Outcome ConsumeAccountRateLimitResetCreditOutcome `json:"outcome"`
}

func (r ConsumeAccountRateLimitResetCreditResponse) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Outcome ConsumeAccountRateLimitResetCreditOutcome `json:"outcome"`
	}{r.Outcome})
}

func (r *ConsumeAccountRateLimitResetCreditResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode consume account rate-limit reset credit response into nil receiver")
	}
	const objectName = "consume account rate-limit reset credit response"
	payload, err := decodeRustSerdeObject(data, objectName, "outcome")
	if err != nil {
		return err
	}
	outcome, err := decodeRequiredThreadItemValue[ConsumeAccountRateLimitResetCreditOutcome](payload, objectName, "outcome")
	if err != nil {
		return err
	}
	*r = ConsumeAccountRateLimitResetCreditResponse{Outcome: outcome}
	return nil
}

// AccountRateLimitsUpdatedNotification is exact standalone rolling-update data.
type AccountRateLimitsUpdatedNotification struct {
	RateLimits RateLimitSnapshot `json:"rateLimits"`
}

func (n AccountRateLimitsUpdatedNotification) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		RateLimits RateLimitSnapshot `json:"rateLimits"`
	}{n.RateLimits})
}

func (n *AccountRateLimitsUpdatedNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode account rate limits updated notification into nil receiver")
	}
	const objectName = "account rate limits updated notification"
	payload, err := decodeRustSerdeObject(data, objectName, "rateLimits")
	if err != nil {
		return err
	}
	rateLimits, err := decodeRequiredThreadItemValue[RateLimitSnapshot](payload, objectName, "rateLimits")
	if err != nil {
		return err
	}
	*n = AccountRateLimitsUpdatedNotification{RateLimits: rateLimits}
	return nil
}

func decodeOptionalNullableRateLimitValue[T any](
	payload map[string]json.RawMessage,
	objectName string,
	fieldName string,
) (T, error) {
	var zero T
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return zero, nil
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return zero, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	return value, nil
}

func accountRateLimitSchemas() map[string]Schema {
	nullableString := Schema{"type": []any{"string", "null"}}
	nullableInt64 := Schema{"type": []any{"integer", "null"}, "format": "int64"}
	nullableRef := func(name string) Schema {
		return Schema{"anyOf": []any{Schema{"$ref": "#/$defs/" + name}, Schema{"type": "null"}}}
	}
	object := func(properties Schema, required ...string) Schema {
		out := Schema{"type": "object", "properties": properties}
		if len(required) != 0 {
			out["required"] = required
		}
		return out
	}
	return map[string]Schema{
		"PlanType": stringEnumSchema(
			"free", "go", "plus", "pro", "prolite", "team",
			"self_serve_business_usage_based", "business",
			"enterprise_cbp_usage_based", "enterprise", "edu", "unknown",
		),
		"RateLimitReachedType": stringEnumSchema(
			"rate_limit_reached", "workspace_owner_credits_depleted",
			"workspace_member_credits_depleted", "workspace_owner_usage_limit_reached",
			"workspace_member_usage_limit_reached",
		),
		"RateLimitResetType":         stringEnumSchema("codexRateLimits", "unknown"),
		"RateLimitResetCreditStatus": stringEnumSchema("available", "redeeming", "redeemed", "unknown"),
		"ConsumeAccountRateLimitResetCreditOutcome": Schema{"oneOf": []any{
			Schema{"description": "A reset credit was consumed and the eligible rate-limit windows were reset.", "enum": []any{"reset"}, "type": "string"},
			Schema{"description": "No current rate-limit window is eligible for a reset.", "enum": []any{"nothingToReset"}, "type": "string"},
			Schema{"description": "The account has no earned reset credits available.", "enum": []any{"noCredit"}, "type": "string"},
			Schema{"description": "The same idempotency key already completed a reset successfully.", "enum": []any{"alreadyRedeemed"}, "type": "string"},
		}},
		"RateLimitWindow": object(Schema{
			"resetsAt": nullableInt64, "usedPercent": Schema{"format": "int32", "type": "integer"},
			"windowDurationMins": nullableInt64,
		}, "usedPercent"),
		"CreditsSnapshot": object(Schema{
			"balance": nullableString, "hasCredits": Schema{"type": "boolean"}, "unlimited": Schema{"type": "boolean"},
		}, "hasCredits", "unlimited"),
		"SpendControlLimitSnapshot": object(Schema{
			"limit": Schema{"type": "string"}, "remainingPercent": Schema{"format": "int32", "type": "integer"},
			"resetsAt": Schema{"format": "int64", "type": "integer"}, "used": Schema{"type": "string"},
		}, "limit", "remainingPercent", "resetsAt", "used"),
		"RateLimitSnapshot": object(Schema{
			"credits": nullableRef("CreditsSnapshot"), "individualLimit": nullableRef("SpendControlLimitSnapshot"),
			"limitId": nullableString, "limitName": nullableString, "planType": nullableRef("PlanType"),
			"primary": nullableRef("RateLimitWindow"), "rateLimitReachedType": nullableRef("RateLimitReachedType"),
			"secondary": nullableRef("RateLimitWindow"),
		}),
		"RateLimitResetCredit": object(Schema{
			"description": Schema{"description": "Backend-provided display description for this credit, or `null` when unavailable.", "type": []any{"string", "null"}},
			"expiresAt":   Schema{"description": "Unix timestamp in seconds when the credit expires, or `null` if it does not expire.", "format": "int64", "type": []any{"integer", "null"}},
			"grantedAt":   Schema{"description": "Unix timestamp in seconds when the credit was granted.", "format": "int64", "type": "integer"},
			"id":          Schema{"description": "Opaque backend identifier for this reset credit.", "type": "string"},
			"resetType":   Schema{"$ref": "#/$defs/RateLimitResetType"}, "status": Schema{"$ref": "#/$defs/RateLimitResetCreditStatus"},
			"title": Schema{"description": "Backend-provided display title for this credit, or `null` when unavailable.", "type": []any{"string", "null"}},
		}, "grantedAt", "id", "resetType", "status"),
		"RateLimitResetCreditsSummary": object(Schema{
			"availableCount": Schema{"format": "int64", "type": "integer"},
			"credits": Schema{
				"description": "Detail rows for available reset credits, when the backend provides them.\n\n`null` means only `availableCount` is known, while an empty array means details were fetched and no available credits were returned. The backend may cap this list, so its length can be less than `availableCount`.",
				"items":       Schema{"$ref": "#/$defs/RateLimitResetCredit"}, "type": []any{"array", "null"},
			},
		}, "availableCount"),
		"GetAccountRateLimitsResponse": object(Schema{
			"rateLimitResetCredits": nullableRef("RateLimitResetCreditsSummary"),
			"rateLimits":            Schema{"allOf": []any{Schema{"$ref": "#/$defs/RateLimitSnapshot"}}, "description": "Backward-compatible single-bucket view; mirrors the historical payload."},
			"rateLimitsByLimitId": Schema{
				"additionalProperties": Schema{"$ref": "#/$defs/RateLimitSnapshot"},
				"description":          "Multi-bucket view keyed by metered `limit_id` (for example, `codex`).", "type": []any{"object", "null"},
			},
		}, "rateLimits"),
		"ConsumeAccountRateLimitResetCreditParams": object(Schema{
			"creditId":       Schema{"description": "Opaque reset-credit identifier to redeem. When omitted, the backend selects the next available credit.", "type": []any{"string", "null"}},
			"idempotencyKey": Schema{"description": "Identifies one logical reset attempt. A UUID is recommended; reuse the same value when retrying that attempt.", "type": "string"},
		}, "idempotencyKey"),
		"ConsumeAccountRateLimitResetCreditResponse": object(Schema{
			"outcome": Schema{"$ref": "#/$defs/ConsumeAccountRateLimitResetCreditOutcome"},
		}, "outcome"),
		"AccountRateLimitsUpdatedNotification": Schema{
			"description": "Sparse rolling rate-limit update.\n\nClients should merge available values into the most recent `account/rateLimits/read` response or refetch that snapshot. Nullable account metadata may be unavailable in a rolling update and does not clear a previously observed value.",
			"properties":  Schema{"rateLimits": Schema{"$ref": "#/$defs/RateLimitSnapshot"}},
			"required":    []string{"rateLimits"}, "type": "object",
		},
	}
}

var (
	_ json.Marshaler   = PlanType("")
	_ json.Unmarshaler = (*PlanType)(nil)
	_ json.Marshaler   = RateLimitReachedType("")
	_ json.Unmarshaler = (*RateLimitReachedType)(nil)
	_ json.Marshaler   = RateLimitResetType("")
	_ json.Unmarshaler = (*RateLimitResetType)(nil)
	_ json.Marshaler   = RateLimitResetCreditStatus("")
	_ json.Unmarshaler = (*RateLimitResetCreditStatus)(nil)
	_ json.Marshaler   = ConsumeAccountRateLimitResetCreditOutcome("")
	_ json.Unmarshaler = (*ConsumeAccountRateLimitResetCreditOutcome)(nil)
	_ json.Marshaler   = RateLimitWindow{}
	_ json.Unmarshaler = (*RateLimitWindow)(nil)
	_ json.Marshaler   = CreditsSnapshot{}
	_ json.Unmarshaler = (*CreditsSnapshot)(nil)
	_ json.Marshaler   = SpendControlLimitSnapshot{}
	_ json.Unmarshaler = (*SpendControlLimitSnapshot)(nil)
	_ json.Marshaler   = RateLimitSnapshot{}
	_ json.Unmarshaler = (*RateLimitSnapshot)(nil)
	_ json.Marshaler   = RateLimitResetCredit{}
	_ json.Unmarshaler = (*RateLimitResetCredit)(nil)
	_ json.Marshaler   = RateLimitResetCreditsSummary{}
	_ json.Unmarshaler = (*RateLimitResetCreditsSummary)(nil)
	_ json.Marshaler   = GetAccountRateLimitsResponse{}
	_ json.Unmarshaler = (*GetAccountRateLimitsResponse)(nil)
	_ json.Marshaler   = ConsumeAccountRateLimitResetCreditParams{}
	_ json.Unmarshaler = (*ConsumeAccountRateLimitResetCreditParams)(nil)
	_ json.Marshaler   = ConsumeAccountRateLimitResetCreditResponse{}
	_ json.Unmarshaler = (*ConsumeAccountRateLimitResetCreditResponse)(nil)
	_ json.Marshaler   = AccountRateLimitsUpdatedNotification{}
	_ json.Unmarshaler = (*AccountRateLimitsUpdatedNotification)(nil)
)
