import type {
  CancelLoginAccountParams,
  CancelLoginAccountResponse,
  CancelLoginAccountStatus,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? (<T>() => T extends B ? 1 : 2) extends
        (<T>() => T extends A ? 1 : 2)
      ? true
      : false
    : false;
type Expect<T extends true> = T;

type ParamsContract = Expect<Equal<CancelLoginAccountParams, { loginId: string }>>;
type StatusContract = Expect<Equal<CancelLoginAccountStatus, "canceled" | "notFound">>;
type ResponseContract = Expect<
  Equal<CancelLoginAccountResponse, { status: CancelLoginAccountStatus }>
>;

export const emptyLoginId = { loginId: "" } satisfies CancelLoginAccountParams;
export const loginId = { loginId: "login-1" } satisfies CancelLoginAccountParams;
export const canceled = "canceled" satisfies CancelLoginAccountStatus;
export const notFound = "notFound" satisfies CancelLoginAccountStatus;
export const response = { status: notFound } satisfies CancelLoginAccountResponse;

// @ts-expect-error loginId is required.
export const rejectMissingLoginId = {} satisfies CancelLoginAccountParams;
// @ts-expect-error loginId is non-null.
export const rejectNullLoginId = { loginId: null } satisfies CancelLoginAccountParams;
// @ts-expect-error loginId is a string.
export const rejectNumericLoginId = { loginId: 1 } satisfies CancelLoginAccountParams;
// @ts-expect-error exact camelCase spelling is required.
export const rejectLoginIdCase = { login_id: "id" } satisfies CancelLoginAccountParams;
// @ts-expect-error params are closed.
export const rejectExtraParam = { loginId: "id", extra: true } satisfies CancelLoginAccountParams;

// @ts-expect-error statuses are closed.
export const rejectUnknownStatus = "other" satisfies CancelLoginAccountStatus;
// @ts-expect-error exact casing is required.
export const rejectStatusCase = "Canceled" satisfies CancelLoginAccountStatus;
// @ts-expect-error empty strings are not statuses.
export const rejectEmptyStatus = "" satisfies CancelLoginAccountStatus;
// @ts-expect-error statuses are non-null.
export const rejectNullStatus = null satisfies CancelLoginAccountStatus;
// @ts-expect-error statuses are strings.
export const rejectNumericStatus = 1 satisfies CancelLoginAccountStatus;

// @ts-expect-error status is required.
export const rejectMissingStatus = {} satisfies CancelLoginAccountResponse;
// @ts-expect-error status is non-null.
export const rejectNullResponseStatus = { status: null } satisfies CancelLoginAccountResponse;
// @ts-expect-error response status is closed.
export const rejectUnknownResponseStatus = { status: "other" } satisfies CancelLoginAccountResponse;
// @ts-expect-error exact response field casing is required.
export const rejectResponseCase = { Status: "canceled" } satisfies CancelLoginAccountResponse;
// @ts-expect-error responses are closed.
export const rejectExtraResponse = { status: "canceled", extra: true } satisfies CancelLoginAccountResponse;

void (null as unknown as ParamsContract);
void (null as unknown as StatusContract);
void (null as unknown as ResponseContract);
