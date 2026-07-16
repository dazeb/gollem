import type {
  ChatgptAuthTokensRefreshParams,
  ChatgptAuthTokensRefreshReason,
  ChatgptAuthTokensRefreshResponse,
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

type Contracts = [
  Expect<Equal<ChatgptAuthTokensRefreshReason, "unauthorized">>,
  Expect<
    Equal<
      ChatgptAuthTokensRefreshParams,
      {
        reason: ChatgptAuthTokensRefreshReason;
        previousAccountId?: string | null;
      }
    >
  >,
  Expect<
    Equal<
      ChatgptAuthTokensRefreshResponse,
      {
        accessToken: string;
        chatgptAccountId: string;
        chatgptPlanType: string | null;
      }
    >
  >,
  ExpectFalse<"account/chatgptAuthTokens/refresh" extends keyof MethodParamsByName ? true : false>,
  ExpectFalse<"account/chatgptAuthTokens/refresh" extends keyof MethodResultsByName ? true : false>,
  Expect<Equal<Extract<MethodParamsByName[keyof MethodParamsByName], ChatgptAuthTokensRefreshParams>, never>>,
  Expect<Equal<Extract<MethodResultsByName[keyof MethodResultsByName], ChatgptAuthTokensRefreshResponse>, never>>,
  Expect<Equal<Extract<ItemPayloadByKind[keyof ItemPayloadByKind], ChatgptAuthTokensRefreshParams>, never>>,
];

export const reason = "unauthorized" satisfies ChatgptAuthTokensRefreshReason;
export const emptyParams = {
  reason: "unauthorized",
} satisfies ChatgptAuthTokensRefreshParams;
export const nullParams = {
  reason: "unauthorized",
  previousAccountId: null,
} satisfies ChatgptAuthTokensRefreshParams;
export const fullParams = {
  reason: "unauthorized",
  previousAccountId: " workspace ",
} satisfies ChatgptAuthTokensRefreshParams;
export const nullResponse = {
  accessToken: "",
  chatgptAccountId: "",
  chatgptPlanType: null,
} satisfies ChatgptAuthTokensRefreshResponse;
export const fullResponse = {
  accessToken: " token ",
  chatgptAccountId: " workspace ",
  chatgptPlanType: " pro ",
} satisfies ChatgptAuthTokensRefreshResponse;

// @ts-expect-error reasons are closed and case-sensitive.
export const rejectReason = "Unauthorized" satisfies ChatgptAuthTokensRefreshReason;
// @ts-expect-error reason is required.
export const rejectMissingReason = {} satisfies ChatgptAuthTokensRefreshParams;
// @ts-expect-error reason is non-null.
export const rejectNullReason = { reason: null } satisfies ChatgptAuthTokensRefreshParams;
// @ts-expect-error previousAccountId is a nullable string.
export const rejectNumericAccount = { reason: "unauthorized", previousAccountId: 1 } satisfies ChatgptAuthTokensRefreshParams;
// @ts-expect-error canonical params are closed.
export const rejectParamsExtension = { reason: "unauthorized", future: true } satisfies ChatgptAuthTokensRefreshParams;
// @ts-expect-error accessToken is required.
export const rejectMissingToken = { chatgptAccountId: "account", chatgptPlanType: null } satisfies ChatgptAuthTokensRefreshResponse;
// @ts-expect-error chatgptAccountId is required.
export const rejectMissingAccount = { accessToken: "token", chatgptPlanType: null } satisfies ChatgptAuthTokensRefreshResponse;
// @ts-expect-error chatgptPlanType is canonical-required nullable.
export const rejectMissingPlan = { accessToken: "token", chatgptAccountId: "account" } satisfies ChatgptAuthTokensRefreshResponse;
// @ts-expect-error accessToken is non-null.
export const rejectNullToken = { accessToken: null, chatgptAccountId: "account", chatgptPlanType: null } satisfies ChatgptAuthTokensRefreshResponse;
// @ts-expect-error chatgptPlanType is a nullable string.
export const rejectNumericPlan = { accessToken: "token", chatgptAccountId: "account", chatgptPlanType: 1 } satisfies ChatgptAuthTokensRefreshResponse;
// @ts-expect-error canonical responses are closed.
export const rejectResponseExtension = { accessToken: "token", chatgptAccountId: "account", chatgptPlanType: null, future: true } satisfies ChatgptAuthTokensRefreshResponse;

void (null as unknown as Contracts);
