import type {
  AgentPath,
  CollabAgentState,
  CollabAgentStatus,
  CollabAgentTool,
  CollabAgentToolCallStatus,
  ReasoningEffort,
  SubAgentActivityKind,
} from "../gollem_appserver_protocol";

export const agentPath = "agent/1" satisfies AgentPath;
export const reasoningEffort = "custom" satisfies ReasoningEffort;
export const collabStatuses = [
  "pendingInit",
  "running",
  "interrupted",
  "completed",
  "errored",
  "shutdown",
  "notFound",
] satisfies CollabAgentStatus[];
export const collabTools = [
  "spawnAgent",
  "sendInput",
  "resumeAgent",
  "wait",
  "closeAgent",
] satisfies CollabAgentTool[];
export const toolCallStatuses = [
  "inProgress",
  "completed",
  "failed",
] satisfies CollabAgentToolCallStatus[];
export const activityKinds = [
  "started",
  "interacted",
  "interrupted",
] satisfies SubAgentActivityKind[];
export const states = [
  { status: "pendingInit", message: null },
  { status: "running", message: "working" },
] satisfies CollabAgentState[];

// @ts-expect-error CollabAgentState message is required nullable.
export const rejectMissingMessage = { status: "running" } satisfies CollabAgentState;
// @ts-expect-error CollabAgentState status is closed.
export const rejectUnknownStatus = { status: "unknown", message: null } satisfies CollabAgentState;
// @ts-expect-error CollabAgentStatus is closed and camelCase.
export const rejectSnakeCaseStatus = "pending_init" satisfies CollabAgentStatus;
// @ts-expect-error CollabAgentTool is closed and camelCase.
export const rejectUnknownTool = "spawn_agent" satisfies CollabAgentTool;
// @ts-expect-error CollabAgentToolCallStatus is closed.
export const rejectUnknownToolStatus = "pending" satisfies CollabAgentToolCallStatus;
// @ts-expect-error SubAgentActivityKind is closed.
export const rejectUnknownActivity = "completed" satisfies SubAgentActivityKind;
