import type {
  AddCreditsNudgeCreditType,
  AddCreditsNudgeEmailStatus,
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
  Expect<Equal<AddCreditsNudgeCreditType, "credits" | "usage_limit">>,
  Expect<Equal<AddCreditsNudgeEmailStatus, "sent" | "cooldown_active">>,
  ExpectFalse<"account/sendAddCreditsNudgeEmail" extends keyof MethodParamsByName ? true : false>,
  ExpectFalse<"account/sendAddCreditsNudgeEmail" extends keyof MethodResultsByName ? true : false>,
  Expect<Equal<Extract<MethodParamsByName[keyof MethodParamsByName], AddCreditsNudgeCreditType | AddCreditsNudgeEmailStatus>, never>>,
  Expect<Equal<Extract<MethodResultsByName[keyof MethodResultsByName], AddCreditsNudgeCreditType | AddCreditsNudgeEmailStatus>, never>>,
  Expect<Equal<Extract<ItemPayloadByKind[keyof ItemPayloadByKind], AddCreditsNudgeCreditType | AddCreditsNudgeEmailStatus>, never>>,
];

export const creditTypes = [
  "credits",
  "usage_limit",
] satisfies AddCreditsNudgeCreditType[];
export const statuses = [
  "sent",
  "cooldown_active",
] satisfies AddCreditsNudgeEmailStatus[];

// @ts-expect-error credit types are closed.
export const rejectUnknownCreditType = "other" satisfies AddCreditsNudgeCreditType;
// @ts-expect-error exact lowercase spelling is required.
export const rejectCreditTypeCase = "Credits" satisfies AddCreditsNudgeCreditType;
// @ts-expect-error snake-case is required.
export const rejectCreditTypeCamelCase = "usageLimit" satisfies AddCreditsNudgeCreditType;
// @ts-expect-error statuses and credit types are distinct.
export const rejectStatusAsCreditType = "sent" satisfies AddCreditsNudgeCreditType;
// @ts-expect-error empty strings are not credit types.
export const rejectEmptyCreditType = "" satisfies AddCreditsNudgeCreditType;
// @ts-expect-error credit types are non-null.
export const rejectNullCreditType = null satisfies AddCreditsNudgeCreditType;
// @ts-expect-error credit types are strings.
export const rejectNumberCreditType = 1 satisfies AddCreditsNudgeCreditType;

// @ts-expect-error email statuses are closed.
export const rejectUnknownStatus = "other" satisfies AddCreditsNudgeEmailStatus;
// @ts-expect-error exact lowercase spelling is required.
export const rejectStatusCase = "Sent" satisfies AddCreditsNudgeEmailStatus;
// @ts-expect-error snake-case is required.
export const rejectStatusCamelCase = "cooldownActive" satisfies AddCreditsNudgeEmailStatus;
// @ts-expect-error credit types and statuses are distinct.
export const rejectCreditTypeAsStatus = "credits" satisfies AddCreditsNudgeEmailStatus;
// @ts-expect-error email statuses are non-null.
export const rejectNullStatus = null satisfies AddCreditsNudgeEmailStatus;
// @ts-expect-error email statuses are strings.
export const rejectNumberStatus = 1 satisfies AddCreditsNudgeEmailStatus;

void (null as unknown as Contracts);
