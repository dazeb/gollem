import type {
  ErrorNotification,
  GuardianWarningNotification,
  TurnError,
  WarningNotification,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type Contracts = [
  Expect<Equal<WarningNotification, {
    threadId: string | null;
    message: string;
  }>>,
  Expect<Equal<GuardianWarningNotification, {
    threadId: string;
    message: string;
  }>>,
  Expect<Equal<ErrorNotification, {
    error: TurnError;
    willRetry: boolean;
    threadId: string;
    turnId: string;
  }>>,
];

export const warning = { threadId: null, message: "" } satisfies WarningNotification;
export const targetedWarning = { threadId: "thread", message: "warn" } satisfies WarningNotification;
export const guardian = { threadId: "thread", message: "guardian" } satisfies GuardianWarningNotification;
export const error = {
  error: { message: "boom", codexErrorInfo: null, additionalDetails: null },
  willRetry: false,
  threadId: "thread",
  turnId: "turn",
} satisfies ErrorNotification;

// @ts-expect-error canonical warnings require explicit nullable threadId.
export const rejectMissingWarningThread = { message: "warn" } satisfies WarningNotification;
// @ts-expect-error warning thread ids are nullable strings only.
export const rejectNumericWarningThread = { threadId: 1, message: "warn" } satisfies WarningNotification;
// @ts-expect-error warning message is required.
export const rejectMissingWarningMessage = { threadId: null } satisfies WarningNotification;
// @ts-expect-error public warnings exclude detail extensions.
export const rejectWarningExtension = { threadId: null, message: "warn", details: null } satisfies WarningNotification;
// @ts-expect-error guardian thread id is required.
export const rejectMissingGuardianThread = { message: "warn" } satisfies GuardianWarningNotification;
// @ts-expect-error guardian thread id is non-null.
export const rejectNullGuardianThread = { threadId: null, message: "warn" } satisfies GuardianWarningNotification;
// @ts-expect-error error is required.
export const rejectMissingError = { willRetry: false, threadId: "thread", turnId: "turn" } satisfies ErrorNotification;
// @ts-expect-error retry state is required.
export const rejectMissingRetry = { error: { message: "boom", codexErrorInfo: null, additionalDetails: null }, threadId: "thread", turnId: "turn" } satisfies ErrorNotification;
// @ts-expect-error nested TurnError remains strict.
export const rejectMalformedError = { error: { message: "boom" }, willRetry: false, threadId: "thread", turnId: "turn" } satisfies ErrorNotification;
// @ts-expect-error error ids are non-null strings.
export const rejectNullErrorThread = { error: { message: "boom", codexErrorInfo: null, additionalDetails: null }, willRetry: false, threadId: null, turnId: "turn" } satisfies ErrorNotification;
// @ts-expect-error live timestamps are not part of the exact error record.
export const rejectErrorExtension = { error: { message: "boom", codexErrorInfo: null, additionalDetails: null }, willRetry: false, threadId: "thread", turnId: "turn", at: "now" } satisfies ErrorNotification;

void (null as unknown as Contracts);
