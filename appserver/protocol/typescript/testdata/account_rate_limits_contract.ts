import type {
  AccountRateLimitsUpdatedNotification,
  ConsumeAccountRateLimitResetCreditOutcome,
  ConsumeAccountRateLimitResetCreditParams,
  ConsumeAccountRateLimitResetCreditResponse,
  CreditsSnapshot,
  GetAccountRateLimitsResponse,
  PlanType,
  RateLimitReachedType,
  RateLimitResetCredit,
  RateLimitResetCreditsSummary,
  RateLimitResetCreditStatus,
  RateLimitResetType,
  RateLimitSnapshot,
  RateLimitWindow,
  SpendControlLimitSnapshot,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2) ? true : false;
type Expect<T extends true> = T;

type Contracts = [
  Expect<Equal<PlanType,
    "free" | "go" | "plus" | "pro" | "prolite" | "team" |
    "self_serve_business_usage_based" | "business" |
    "enterprise_cbp_usage_based" | "enterprise" | "edu" | "unknown">>,
  Expect<Equal<RateLimitReachedType,
    "rate_limit_reached" | "workspace_owner_credits_depleted" |
    "workspace_member_credits_depleted" | "workspace_owner_usage_limit_reached" |
    "workspace_member_usage_limit_reached">>,
  Expect<Equal<RateLimitResetType, "codexRateLimits" | "unknown">>,
  Expect<Equal<RateLimitResetCreditStatus, "available" | "redeeming" | "redeemed" | "unknown">>,
  Expect<Equal<ConsumeAccountRateLimitResetCreditOutcome,
    "reset" | "nothingToReset" | "noCredit" | "alreadyRedeemed">>,
  Expect<Equal<RateLimitWindow, {
    usedPercent: number;
    windowDurationMins: number | null;
    resetsAt: number | null;
  }>>,
  Expect<Equal<CreditsSnapshot, {
    hasCredits: boolean;
    unlimited: boolean;
    balance: string | null;
  }>>,
  Expect<Equal<SpendControlLimitSnapshot, {
    limit: string;
    used: string;
    remainingPercent: number;
    resetsAt: number;
  }>>,
  Expect<Equal<RateLimitSnapshot, {
    limitId: string | null;
    limitName: string | null;
    primary: RateLimitWindow | null;
    secondary: RateLimitWindow | null;
    credits: CreditsSnapshot | null;
    individualLimit: SpendControlLimitSnapshot | null;
    planType: PlanType | null;
    rateLimitReachedType: RateLimitReachedType | null;
  }>>,
  Expect<Equal<RateLimitResetCredit, {
    id: string;
    resetType: RateLimitResetType;
    status: RateLimitResetCreditStatus;
    grantedAt: number;
    expiresAt: number | null;
    title: string | null;
    description: string | null;
  }>>,
  Expect<Equal<RateLimitResetCreditsSummary, {
    availableCount: bigint;
    credits: Array<RateLimitResetCredit> | null;
  }>>,
  Expect<Equal<GetAccountRateLimitsResponse, {
    rateLimits: RateLimitSnapshot;
    rateLimitsByLimitId: { [key in string]?: RateLimitSnapshot } | null;
    rateLimitResetCredits: RateLimitResetCreditsSummary | null;
  }>>,
  Expect<Equal<ConsumeAccountRateLimitResetCreditParams, {
    idempotencyKey: string;
    creditId?: string | null;
  }>>,
  Expect<Equal<ConsumeAccountRateLimitResetCreditResponse, {
    outcome: ConsumeAccountRateLimitResetCreditOutcome;
  }>>,
  Expect<Equal<AccountRateLimitsUpdatedNotification, {
    rateLimits: RateLimitSnapshot;
  }>>,
];

const sparse: RateLimitSnapshot = {
  limitId: null,
  limitName: null,
  primary: null,
  secondary: null,
  credits: null,
  individualLimit: null,
  planType: null,
  rateLimitReachedType: null,
};
({ usedPercent: 0, windowDurationMins: null, resetsAt: null }) satisfies RateLimitWindow;
({ hasCredits: false, unlimited: true, balance: null }) satisfies CreditsSnapshot;
({ limit: "", used: "", remainingPercent: -1, resetsAt: 0 }) satisfies SpendControlLimitSnapshot;
({ availableCount: 0n, credits: null }) satisfies RateLimitResetCreditsSummary;
({ rateLimits: sparse, rateLimitsByLimitId: { codex: sparse }, rateLimitResetCredits: null }) satisfies GetAccountRateLimitsResponse;
({ idempotencyKey: "key" }) satisfies ConsumeAccountRateLimitResetCreditParams;
({ idempotencyKey: "key", creditId: null }) satisfies ConsumeAccountRateLimitResetCreditParams;
({ outcome: "reset" }) satisfies ConsumeAccountRateLimitResetCreditResponse;
({ rateLimits: sparse }) satisfies AccountRateLimitsUpdatedNotification;

// @ts-expect-error plan labels are exact.
"future" satisfies PlanType;
// @ts-expect-error reached reasons are exact.
"unknown" satisfies RateLimitReachedType;
// @ts-expect-error reset types are exact.
"codex_rate_limits" satisfies RateLimitResetType;
// @ts-expect-error reset statuses are exact.
"pending" satisfies RateLimitResetCreditStatus;
// @ts-expect-error outcomes are camel-case literals.
"nothing_to_reset" satisfies ConsumeAccountRateLimitResetCreditOutcome;
// @ts-expect-error nullable window fields remain required.
({ usedPercent: 0 }) satisfies RateLimitWindow;
// @ts-expect-error balance remains required.
({ hasCredits: false, unlimited: false }) satisfies CreditsSnapshot;
// @ts-expect-error resetsAt is numeric.
({ limit: "1", used: "0", remainingPercent: 100, resetsAt: 0n }) satisfies SpendControlLimitSnapshot;
// @ts-expect-error sparse fields remain explicit in TypeScript.
({}) satisfies RateLimitSnapshot;
// @ts-expect-error reset-credit nullable metadata remains explicit.
({ id: "c", resetType: "unknown", status: "unknown", grantedAt: 0 }) satisfies RateLimitResetCredit;
// @ts-expect-error availableCount is bigint.
({ availableCount: 0, credits: null }) satisfies RateLimitResetCreditsSummary;
// @ts-expect-error response nullable views remain required.
({ rateLimits: sparse }) satisfies GetAccountRateLimitsResponse;
// @ts-expect-error idempotencyKey is required.
({ creditId: null }) satisfies ConsumeAccountRateLimitResetCreditParams;
// @ts-expect-error outcomes are exact.
({ outcome: "unknown" }) satisfies ConsumeAccountRateLimitResetCreditResponse;
// @ts-expect-error rolling notification is closed.
({ rateLimits: sparse, future: true }) satisfies AccountRateLimitsUpdatedNotification;

void (null as unknown as Contracts);
