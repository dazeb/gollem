import type {
  McpServerStartupFailureReason,
  McpServerStartupState,
  McpServerStatusUpdatedNotification,
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

type NotificationContract = Expect<
  Equal<
    McpServerStatusUpdatedNotification,
    {
      error: string | null;
      failureReason: McpServerStartupFailureReason | null;
      name: string;
      status: McpServerStartupState;
      threadId: string | null;
    }
  >
>;

export const starting: McpServerStatusUpdatedNotification = {
  error: null,
  failureReason: null,
  name: "server",
  status: "starting",
  threadId: null,
};

export const failed: McpServerStatusUpdatedNotification = {
  error: "login required",
  failureReason: "reauthenticationRequired",
  name: "server",
  status: "failed",
  threadId: "thread",
};

// @ts-expect-error name is required.
export const rejectMissingName: McpServerStatusUpdatedNotification = {
  error: null,
  failureReason: null,
  status: "ready",
  threadId: null,
};
// @ts-expect-error status is required.
export const rejectMissingStatus: McpServerStatusUpdatedNotification = {
  error: null,
  failureReason: null,
  name: "server",
  threadId: null,
};
// @ts-expect-error canonical generated values require threadId.
export const rejectMissingThreadId: McpServerStatusUpdatedNotification = {
  error: null,
  failureReason: null,
  name: "server",
  status: "ready",
};
// @ts-expect-error canonical generated values require error.
export const rejectMissingError: McpServerStatusUpdatedNotification = {
  failureReason: null,
  name: "server",
  status: "ready",
  threadId: null,
};
// @ts-expect-error canonical generated values require failureReason.
export const rejectMissingFailureReason: McpServerStatusUpdatedNotification = {
  error: null,
  name: "server",
  status: "ready",
  threadId: null,
};
export const rejectNullName: McpServerStatusUpdatedNotification = {
  error: null,
  failureReason: null,
  // @ts-expect-error name is non-null.
  name: null,
  status: "ready",
  threadId: null,
};
export const rejectUnknownStatus: McpServerStatusUpdatedNotification = {
  error: null,
  failureReason: null,
  name: "server",
  // @ts-expect-error startup states are closed.
  status: "other",
  threadId: null,
};
export const rejectUnknownReason: McpServerStatusUpdatedNotification = {
  error: null,
  // @ts-expect-error startup failure reasons are closed.
  failureReason: "other",
  name: "server",
  status: "failed",
  threadId: null,
};
export const rejectNumberThreadId: McpServerStatusUpdatedNotification = {
  error: null,
  failureReason: null,
  name: "server",
  status: "ready",
  // @ts-expect-error threadId is nullable string.
  threadId: 1,
};
export const rejectNumberError: McpServerStatusUpdatedNotification = {
  // @ts-expect-error error is nullable string.
  error: 1,
  failureReason: null,
  name: "server",
  status: "failed",
  threadId: null,
};
export const rejectExtra: McpServerStatusUpdatedNotification = {
  error: null,
  failureReason: null,
  name: "server",
  status: "ready",
  threadId: null,
  // @ts-expect-error generated object is closed.
  extra: true,
};

void (null as unknown as NotificationContract);
