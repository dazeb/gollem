import type {
  AccountTokenUsageDailyBucket,
  AccountTokenUsageSummary,
  ItemPayloadByKind,
  MethodParamsByName,
  MethodResultsByName,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;
type ExpectFalse<T extends false> = T;

type ExactDailyBucket = {
  startDate: string;
  tokens: bigint;
};
type ExactSummary = {
  lifetimeTokens: bigint | null;
  peakDailyTokens: bigint | null;
  longestRunningTurnSec: bigint | null;
  currentStreakDays: bigint | null;
  longestStreakDays: bigint | null;
};

type Contracts = [
  Expect<Equal<AccountTokenUsageDailyBucket, ExactDailyBucket>>,
  Expect<Equal<AccountTokenUsageSummary, ExactSummary>>,
  ExpectFalse<"account/usage/read" extends keyof MethodParamsByName ? true : false>,
  ExpectFalse<"account/usage/read" extends keyof MethodResultsByName ? true : false>,
  Expect<Equal<Extract<MethodParamsByName[keyof MethodParamsByName], AccountTokenUsageDailyBucket | AccountTokenUsageSummary>, never>>,
  Expect<Equal<Extract<MethodResultsByName[keyof MethodResultsByName], AccountTokenUsageDailyBucket | AccountTokenUsageSummary>, never>>,
  Expect<Equal<Extract<ItemPayloadByKind[keyof ItemPayloadByKind], AccountTokenUsageDailyBucket | AccountTokenUsageSummary>, never>>,
];

export const minimumBucket = {
  startDate: "",
  tokens: -9223372036854775808n,
} satisfies AccountTokenUsageDailyBucket;
export const maximumBucket = {
  startDate: " arbitrary ",
  tokens: 9223372036854775807n,
} satisfies AccountTokenUsageDailyBucket;
export const nullSummary = {
  lifetimeTokens: null,
  peakDailyTokens: null,
  longestRunningTurnSec: null,
  currentStreakDays: null,
  longestStreakDays: null,
} satisfies AccountTokenUsageSummary;
export const populatedSummary = {
  lifetimeTokens: -9223372036854775808n,
  peakDailyTokens: 9223372036854775807n,
  longestRunningTurnSec: 0n,
  currentStreakDays: -1n,
  longestStreakDays: 1n,
} satisfies AccountTokenUsageSummary;

// @ts-expect-error startDate is required.
export const rejectBucketMissingDate = { tokens: 0n } satisfies AccountTokenUsageDailyBucket;
// @ts-expect-error tokens is required.
export const rejectBucketMissingTokens = { startDate: "date" } satisfies AccountTokenUsageDailyBucket;
// @ts-expect-error startDate is non-null.
export const rejectBucketNullDate = { startDate: null, tokens: 0n } satisfies AccountTokenUsageDailyBucket;
// @ts-expect-error tokens is bigint in exact ts-rs output.
export const rejectBucketNumberTokens = { startDate: "date", tokens: 0 } satisfies AccountTokenUsageDailyBucket;
// @ts-expect-error canonical buckets are closed.
export const rejectBucketExtension = { startDate: "date", tokens: 0n, future: true } satisfies AccountTokenUsageDailyBucket;

// @ts-expect-error lifetimeTokens is canonical-required nullable.
export const rejectSummaryMissingLifetime = { peakDailyTokens: null, longestRunningTurnSec: null, currentStreakDays: null, longestStreakDays: null } satisfies AccountTokenUsageSummary;
// @ts-expect-error peakDailyTokens is canonical-required nullable.
export const rejectSummaryMissingPeak = { lifetimeTokens: null, longestRunningTurnSec: null, currentStreakDays: null, longestStreakDays: null } satisfies AccountTokenUsageSummary;
// @ts-expect-error longestRunningTurnSec is canonical-required nullable.
export const rejectSummaryMissingLongestTurn = { lifetimeTokens: null, peakDailyTokens: null, currentStreakDays: null, longestStreakDays: null } satisfies AccountTokenUsageSummary;
// @ts-expect-error currentStreakDays is canonical-required nullable.
export const rejectSummaryMissingCurrentStreak = { lifetimeTokens: null, peakDailyTokens: null, longestRunningTurnSec: null, longestStreakDays: null } satisfies AccountTokenUsageSummary;
// @ts-expect-error longestStreakDays is canonical-required nullable.
export const rejectSummaryMissingLongestStreak = { lifetimeTokens: null, peakDailyTokens: null, longestRunningTurnSec: null, currentStreakDays: null } satisfies AccountTokenUsageSummary;
// @ts-expect-error summary integers are bigint, not number.
export const rejectSummaryNumber = { ...nullSummary, lifetimeTokens: 0 } satisfies AccountTokenUsageSummary;
// @ts-expect-error summary values are bigint or null.
export const rejectSummaryString = { ...nullSummary, peakDailyTokens: "0" } satisfies AccountTokenUsageSummary;
// @ts-expect-error canonical summaries are closed.
export const rejectSummaryExtension = { ...nullSummary, future: true } satisfies AccountTokenUsageSummary;

void (null as unknown as Contracts);
