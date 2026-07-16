import type {
  AppTemplateUnavailableReason,
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

type Contracts = [
  Expect<Equal<AppTemplateUnavailableReason, "NOT_CONFIGURED_FOR_WORKSPACE" | "NO_ACTIVE_WORKSPACE">>,
  Expect<Equal<Extract<MethodParamsByName[keyof MethodParamsByName], AppTemplateUnavailableReason>, never>>,
  Expect<Equal<Extract<MethodResultsByName[keyof MethodResultsByName], AppTemplateUnavailableReason>, never>>,
  Expect<Equal<Extract<ItemPayloadByKind[keyof ItemPayloadByKind], AppTemplateUnavailableReason>, never>>,
];

export const reasons = [
  "NOT_CONFIGURED_FOR_WORKSPACE",
  "NO_ACTIVE_WORKSPACE",
] satisfies AppTemplateUnavailableReason[];

// @ts-expect-error reasons are closed.
export const rejectUnknown = "other" satisfies AppTemplateUnavailableReason;
// @ts-expect-error exact uppercase spelling is required.
export const rejectLowercase = "not_configured_for_workspace" satisfies AppTemplateUnavailableReason;
// @ts-expect-error screaming snake case is required.
export const rejectCamelCase = "NotConfiguredForWorkspace" satisfies AppTemplateUnavailableReason;
// @ts-expect-error whitespace is significant.
export const rejectTrailingWhitespace = "NO_ACTIVE_WORKSPACE " satisfies AppTemplateUnavailableReason;
// @ts-expect-error empty strings are not reasons.
export const rejectEmpty = "" satisfies AppTemplateUnavailableReason;
// @ts-expect-error reasons are non-null.
export const rejectNull = null satisfies AppTemplateUnavailableReason;
// @ts-expect-error reasons are strings.
export const rejectNumber = 1 satisfies AppTemplateUnavailableReason;
// @ts-expect-error reasons are strings.
export const rejectBoolean = true satisfies AppTemplateUnavailableReason;
// @ts-expect-error reasons are strings.
export const rejectObject = {} satisfies AppTemplateUnavailableReason;

void (null as unknown as Contracts);
