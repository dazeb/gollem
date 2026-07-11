import type {
  CommandExecutionOutputDeltaNotification,
  CommandExecutionOutputDeltaNotificationParams,
  CommandExecutionStatus,
  DynamicToolCallStatus,
  FileChangePatchUpdatedNotification,
  FileChangePatchUpdatedNotificationParams,
  MCPToolCallError,
  MCPToolCallProgressNotificationParams,
  McpToolCallError,
  McpToolCallProgressNotification,
  McpToolCallStatus,
  ParamsFor,
  RequestId,
  ServerRequestResolvedNotification,
  ServerRequestResolvedNotificationParams,
} from "../gollem_appserver_protocol";

export const stringRequestId = "request-1" satisfies RequestId;
export const numericRequestId = 42 satisfies RequestId;

export const commandStatuses = [
  "inProgress",
  "completed",
  "failed",
  "declined",
] as const satisfies readonly CommandExecutionStatus[];
export const dynamicStatuses = [
  "inProgress",
  "completed",
  "failed",
] as const satisfies readonly DynamicToolCallStatus[];
export const mcpStatuses = [
  "inProgress",
  "completed",
  "failed",
] as const satisfies readonly McpToolCallStatus[];

export const commandOutput = {
  threadId: "thread-1",
  turnId: "turn-1",
  itemId: "item-command",
  delta: "ok\n",
} satisfies CommandExecutionOutputDeltaNotification;
commandOutput satisfies ParamsFor<"item/commandExecution/outputDelta">;
commandOutput satisfies CommandExecutionOutputDeltaNotificationParams;

export const filePatch = {
  threadId: "thread-1",
  turnId: "turn-1",
  itemId: "item-file",
  changes: [],
} satisfies FileChangePatchUpdatedNotification;
filePatch satisfies ParamsFor<"item/fileChange/patchUpdated">;
filePatch satisfies FileChangePatchUpdatedNotificationParams;

export const mcpError = { message: "failed" } satisfies McpToolCallError;
mcpError satisfies MCPToolCallError;
export const mcpProgress = {
  threadId: "thread-1",
  turnId: "turn-1",
  itemId: "item-mcp",
  message: "Searching repository",
} satisfies McpToolCallProgressNotification;
mcpProgress satisfies ParamsFor<"item/mcpToolCall/progress">;
mcpProgress satisfies MCPToolCallProgressNotificationParams;

export const resolvedString = {
  threadId: "thread-1",
  requestId: stringRequestId,
} satisfies ServerRequestResolvedNotification;
export const resolvedNumber = {
  threadId: "thread-1",
  requestId: numericRequestId,
} satisfies ServerRequestResolvedNotification;
resolvedString satisfies ParamsFor<"serverRequest/resolved">;
resolvedNumber satisfies ServerRequestResolvedNotificationParams;

// @ts-expect-error request ids are strings or numbers.
true satisfies RequestId;
// @ts-expect-error command statuses are closed.
"cancelled" satisfies CommandExecutionStatus;
// @ts-expect-error dynamic-tool statuses do not include declined.
"declined" satisfies DynamicToolCallStatus;
// @ts-expect-error MCP-tool statuses do not include declined.
"declined" satisfies McpToolCallStatus;
// @ts-expect-error command deltas require delta.
({ threadId: "thread-1", turnId: "turn-1", itemId: "item-command" }) satisfies CommandExecutionOutputDeltaNotification;
// @ts-expect-error file patches require a non-null changes array.
({ threadId: "thread-1", turnId: "turn-1", itemId: "item-file", changes: null }) satisfies FileChangePatchUpdatedNotification;
// @ts-expect-error MCP errors require a message.
({}) satisfies McpToolCallError;
// @ts-expect-error MCP progress requires a message.
({ threadId: "thread-1", turnId: "turn-1", itemId: "item-mcp" }) satisfies McpToolCallProgressNotification;
// @ts-expect-error resolved notifications require a request id.
({ threadId: "thread-1" }) satisfies ServerRequestResolvedNotification;
