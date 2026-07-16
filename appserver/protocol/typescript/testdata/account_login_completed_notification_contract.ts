import type {
  AccountLoginCompletedNotification,
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

type ExactNotification = {
  loginId: string | null;
  success: boolean;
  error: string | null;
};

type Contracts = [
  Expect<Equal<AccountLoginCompletedNotification, ExactNotification>>,
  ExpectFalse<"account/login/completed" extends keyof MethodParamsByName ? true : false>,
  ExpectFalse<"account/login/completed" extends keyof MethodResultsByName ? true : false>,
  Expect<Equal<Extract<MethodParamsByName[keyof MethodParamsByName], AccountLoginCompletedNotification>, never>>,
  Expect<Equal<Extract<MethodResultsByName[keyof MethodResultsByName], AccountLoginCompletedNotification>, never>>,
  Expect<Equal<Extract<ItemPayloadByKind[keyof ItemPayloadByKind], AccountLoginCompletedNotification>, never>>,
];

export const nullResult = {
  loginId: null,
  success: true,
  error: null,
} satisfies AccountLoginCompletedNotification;
export const emptyResult = {
  loginId: "",
  success: false,
  error: "",
} satisfies AccountLoginCompletedNotification;
export const arbitraryResult = {
  loginId: " 550e8400-e29b-41d4-a716-446655440000 ",
  success: false,
  error: " denied ",
} satisfies AccountLoginCompletedNotification;

// @ts-expect-error loginId is canonical-required nullable.
export const rejectMissingLoginId = { success: true, error: null } satisfies AccountLoginCompletedNotification;
// @ts-expect-error success is required.
export const rejectMissingSuccess = { loginId: null, error: null } satisfies AccountLoginCompletedNotification;
// @ts-expect-error error is canonical-required nullable.
export const rejectMissingError = { loginId: null, success: true } satisfies AccountLoginCompletedNotification;
// @ts-expect-error loginId is a nullable string.
export const rejectNumericLoginId = { loginId: 1, success: true, error: null } satisfies AccountLoginCompletedNotification;
// @ts-expect-error success is non-null boolean.
export const rejectNullSuccess = { loginId: null, success: null, error: null } satisfies AccountLoginCompletedNotification;
// @ts-expect-error success is boolean.
export const rejectStringSuccess = { loginId: null, success: "true", error: null } satisfies AccountLoginCompletedNotification;
// @ts-expect-error error is a nullable string.
export const rejectNumericError = { loginId: null, success: true, error: 1 } satisfies AccountLoginCompletedNotification;
// @ts-expect-error canonical notifications are closed.
export const rejectExtension = { loginId: null, success: true, error: null, future: true } satisfies AccountLoginCompletedNotification;

void (null as unknown as Contracts);
