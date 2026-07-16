import type {
  Account,
  AccountTokenUsageDailyBucket,
  AccountTokenUsageSummary,
  AccountUpdatedNotification,
  AddCreditsNudgeCreditType,
  AddCreditsNudgeEmailStatus,
  AmazonBedrockCredentialSource,
  AuthMode,
  GetAccountParams,
  GetAccountResponse,
  GetAccountTokenUsageResponse,
  PlanType,
  SendAddCreditsNudgeEmailParams,
  SendAddCreditsNudgeEmailResponse,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2) ? true : false;
type Expect<T extends true> = T;

type ExpectedAccount =
  | { type: "apiKey" }
  | { type: "chatgpt"; email: string | null; planType: PlanType }
  | { type: "amazonBedrock"; credentialSource: AmazonBedrockCredentialSource };

type Contracts = [
  Expect<Equal<Account, ExpectedAccount>>,
  Expect<Equal<AccountUpdatedNotification, {
    authMode: AuthMode | null;
    planType: PlanType | null;
  }>>,
  Expect<Equal<GetAccountParams, { refreshToken?: boolean }>>,
  Expect<Equal<GetAccountResponse, {
    account: Account | null;
    requiresOpenaiAuth: boolean;
  }>>,
  Expect<Equal<GetAccountTokenUsageResponse, {
    dailyUsageBuckets: Array<AccountTokenUsageDailyBucket> | null;
    summary: AccountTokenUsageSummary;
  }>>,
  Expect<Equal<SendAddCreditsNudgeEmailParams, {
    creditType: AddCreditsNudgeCreditType;
  }>>,
  Expect<Equal<SendAddCreditsNudgeEmailResponse, {
    status: AddCreditsNudgeEmailStatus;
  }>>,
];

({ type: "apiKey" }) satisfies Account;
({ type: "chatgpt", email: null, planType: "unknown" }) satisfies Account;
({ type: "amazonBedrock", credentialSource: "awsManaged" }) satisfies Account;
({ authMode: null, planType: null }) satisfies AccountUpdatedNotification;
({}) satisfies GetAccountParams;
({ refreshToken: true }) satisfies GetAccountParams;
({ account: null, requiresOpenaiAuth: false }) satisfies GetAccountResponse;
({ summary: {
  lifetimeTokens: null,
  peakDailyTokens: null,
  longestRunningTurnSec: null,
  currentStreakDays: null,
  longestStreakDays: null,
}, dailyUsageBuckets: [] }) satisfies GetAccountTokenUsageResponse;
({ creditType: "credits" }) satisfies SendAddCreditsNudgeEmailParams;
({ status: "sent" }) satisfies SendAddCreditsNudgeEmailResponse;

// @ts-expect-error account discriminants are exact.
({ type: "api_key" }) satisfies Account;
// @ts-expect-error chatgpt email remains explicit and nullable.
({ type: "chatgpt", planType: "free" }) satisfies Account;
// @ts-expect-error chatgpt plan labels are exact.
({ type: "chatgpt", email: null, planType: "future" }) satisfies Account;
// @ts-expect-error Bedrock credential source is required in TypeScript.
({ type: "amazonBedrock" }) satisfies Account;
// @ts-expect-error account updates require both nullable fields.
({ authMode: null }) satisfies AccountUpdatedNotification;
// @ts-expect-error refresh is boolean.
({ refreshToken: "true" }) satisfies GetAccountParams;
// @ts-expect-error response account is explicit.
({ requiresOpenaiAuth: false }) satisfies GetAccountResponse;
// @ts-expect-error usage buckets are explicit.
({ summary: {} as AccountTokenUsageSummary }) satisfies GetAccountTokenUsageResponse;
// @ts-expect-error nudge credit labels are exact.
({ creditType: "usageLimit" }) satisfies SendAddCreditsNudgeEmailParams;
// @ts-expect-error nudge statuses are exact.
({ status: "cooldownActive" }) satisfies SendAddCreditsNudgeEmailResponse;

void (null as unknown as Contracts);
