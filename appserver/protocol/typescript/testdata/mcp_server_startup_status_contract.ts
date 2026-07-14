import type {
  McpServerStartupFailureReason,
  McpServerStartupState,
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

type FailureReasonContract = Expect<
  Equal<McpServerStartupFailureReason, "reauthenticationRequired">
>;
type StateContract = Expect<
  Equal<
    McpServerStartupState,
    "starting" | "ready" | "failed" | "cancelled"
  >
>;

export const failureReasons = [
  "reauthenticationRequired",
] satisfies McpServerStartupFailureReason[];
export const states = [
  "starting",
  "ready",
  "failed",
  "cancelled",
] satisfies McpServerStartupState[];

// @ts-expect-error startup failure reasons are closed.
export const rejectUnknownReason = "other" satisfies McpServerStartupFailureReason;
// @ts-expect-error exact lower-camel spelling is required.
export const rejectReasonCase = "ReauthenticationRequired" satisfies McpServerStartupFailureReason;
// @ts-expect-error snake-case failure reasons are not accepted.
export const rejectSnakeReason = "reauthentication_required" satisfies McpServerStartupFailureReason;
// @ts-expect-error empty strings are not startup failure reasons.
export const rejectEmptyReason = "" satisfies McpServerStartupFailureReason;
// @ts-expect-error startup failure reasons are non-null.
export const rejectNullReason = null satisfies McpServerStartupFailureReason;
// @ts-expect-error startup failure reasons are strings.
export const rejectNumberReason = 1 satisfies McpServerStartupFailureReason;

// @ts-expect-error startup states are closed.
export const rejectUnknownState = "other" satisfies McpServerStartupState;
// @ts-expect-error exact lowercase spelling is required.
export const rejectStateCase = "Starting" satisfies McpServerStartupState;
// @ts-expect-error American spelling is not the public wire value.
export const rejectCanceledState = "canceled" satisfies McpServerStartupState;
// @ts-expect-error failure reasons and startup states are distinct.
export const rejectFailureAsState = "reauthenticationRequired" satisfies McpServerStartupState;
// @ts-expect-error empty strings are not startup states.
export const rejectEmptyState = "" satisfies McpServerStartupState;
// @ts-expect-error startup states are non-null.
export const rejectNullState = null satisfies McpServerStartupState;
// @ts-expect-error startup states are strings.
export const rejectNumberState = 1 satisfies McpServerStartupState;

void (null as unknown as FailureReasonContract);
void (null as unknown as StateContract);
