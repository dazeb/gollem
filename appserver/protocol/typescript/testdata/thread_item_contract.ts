import type {
  AbsolutePathBuf,
  CollabAgentState,
  CollabAgentTool,
  CollabAgentToolCallStatus,
  CommandAction,
  CommandExecutionSource,
  CommandExecutionStatus,
  DynamicToolCallOutputContentItem,
  DynamicToolCallStatus,
  FileUpdateChange,
  HookPromptFragment,
  JsonValue,
  LegacyAppPathString,
  McpToolCallAppContext,
  McpToolCallError,
  McpToolCallResult,
  McpToolCallStatus,
  MemoryCitation,
  MessagePhase,
  PatchApplyStatus,
  ReasoningEffort,
  SubAgentActivityKind,
  ThreadItem,
  UserInput,
  WebSearchAction,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type ExpectedThreadItem =
  | { type: "userMessage"; id: string; clientId: string | null; content: UserInput[] }
  | { type: "hookPrompt"; id: string; fragments: HookPromptFragment[] }
  | {
      type: "agentMessage";
      id: string;
      text: string;
      phase: MessagePhase | null;
      memoryCitation: MemoryCitation | null;
    }
  | { type: "plan"; id: string; text: string }
  | { type: "reasoning"; id: string; summary: string[]; content: string[] }
  | {
      type: "commandExecution";
      id: string;
      command: string;
      cwd: LegacyAppPathString;
      processId: string | null;
      source: CommandExecutionSource;
      status: CommandExecutionStatus;
      commandActions: CommandAction[];
      aggregatedOutput: string | null;
      exitCode: number | null;
      durationMs: number | null;
    }
  | { type: "fileChange"; id: string; changes: FileUpdateChange[]; status: PatchApplyStatus }
  | {
      type: "mcpToolCall";
      id: string;
      server: string;
      tool: string;
      status: McpToolCallStatus;
      arguments: JsonValue;
      appContext: McpToolCallAppContext | null;
      mcpAppResourceUri?: string;
      pluginId: string | null;
      result: McpToolCallResult | null;
      error: McpToolCallError | null;
      durationMs: number | null;
    }
  | {
      type: "dynamicToolCall";
      id: string;
      namespace: string | null;
      tool: string;
      arguments: JsonValue;
      status: DynamicToolCallStatus;
      contentItems: DynamicToolCallOutputContentItem[] | null;
      success: boolean | null;
      durationMs: number | null;
    }
  | {
      type: "collabAgentToolCall";
      id: string;
      tool: CollabAgentTool;
      status: CollabAgentToolCallStatus;
      senderThreadId: string;
      receiverThreadIds: string[];
      prompt: string | null;
      model: string | null;
      reasoningEffort: ReasoningEffort | null;
      agentsStates: { [key in string]?: CollabAgentState };
    }
  | {
      type: "subAgentActivity";
      id: string;
      kind: SubAgentActivityKind;
      agentThreadId: string;
      agentPath: string;
    }
  | { type: "webSearch"; id: string; query: string; action: WebSearchAction | null }
  | { type: "imageView"; id: string; path: LegacyAppPathString }
  | { type: "sleep"; id: string; durationMs: number }
  | {
      type: "imageGeneration";
      id: string;
      status: string;
      revisedPrompt: string | null;
      result: string;
      savedPath?: AbsolutePathBuf;
    }
  | { type: "enteredReviewMode"; id: string; review: string }
  | { type: "exitedReviewMode"; id: string; review: string }
  | { type: "contextCompaction"; id: string };

type ThreadItemIsExact = Expect<Equal<ThreadItem, ExpectedThreadItem>>;

export const items = [
  {
    type: "userMessage",
    id: "user-1",
    clientId: null,
    content: [{ type: "text", text: "hello", text_elements: [] }],
  },
  {
    type: "hookPrompt",
    id: "hook-1",
    fragments: [{ text: "prompt", hookRunId: "run-1" }],
  },
  {
    type: "agentMessage",
    id: "agent-1",
    text: "done",
    phase: "final_answer",
    memoryCitation: null,
  },
  { type: "plan", id: "plan-1", text: "ship" },
  { type: "reasoning", id: "reason-1", summary: [], content: [] },
  {
    type: "commandExecution",
    id: "command-1",
    command: "pwd",
    cwd: "/workspace",
    processId: null,
    source: "unifiedExecStartup",
    status: "completed",
    commandActions: [{ type: "unknown", command: "pwd" }],
    aggregatedOutput: "ok",
    exitCode: 0,
    durationMs: 1,
  },
  {
    type: "fileChange",
    id: "file-1",
    changes: [{ path: "file.txt", kind: { type: "update", move_path: null }, diff: "+new" }],
    status: "failed",
  },
  {
    type: "mcpToolCall",
    id: "mcp-1",
    server: "docs",
    tool: "search",
    status: "completed",
    arguments: { query: "gollem" },
    appContext: null,
    pluginId: null,
    result: null,
    error: null,
    durationMs: null,
  },
  {
    type: "dynamicToolCall",
    id: "dynamic-1",
    namespace: null,
    tool: "render",
    arguments: null,
    status: "inProgress",
    contentItems: [{ type: "inputText", text: "hello" }],
    success: null,
    durationMs: null,
  },
  {
    type: "collabAgentToolCall",
    id: "collab-1",
    tool: "spawnAgent",
    status: "inProgress",
    senderThreadId: "thread-1",
    receiverThreadIds: ["thread-2"],
    prompt: null,
    model: null,
    reasoningEffort: null,
    agentsStates: { "thread-2": { status: "running", message: null } },
  },
  {
    type: "subAgentActivity",
    id: "activity-1",
    kind: "started",
    agentThreadId: "thread-2",
    agentPath: "agent/path",
  },
  { type: "webSearch", id: "web-1", query: "gollem", action: null },
  { type: "imageView", id: "view-1", path: "relative.png" },
  { type: "sleep", id: "sleep-1", durationMs: 1000 },
  {
    type: "imageGeneration",
    id: "image-1",
    status: "completed",
    revisedPrompt: null,
    result: "base64",
    savedPath: "/workspace/image.png",
  },
  { type: "enteredReviewMode", id: "review-1", review: "review" },
  { type: "exitedReviewMode", id: "review-2", review: "done" },
  { type: "contextCompaction", id: "compact-1" },
] satisfies ThreadItem[];

// @ts-expect-error clientId is required nullable, not optional.
export const rejectMissingClientId = { type: "userMessage", id: "u", content: [] } satisfies ThreadItem;
// @ts-expect-error hook fragments are required.
export const rejectMissingFragments = { type: "hookPrompt", id: "h" } satisfies ThreadItem;
// @ts-expect-error phase is required nullable, not optional.
export const rejectMissingAgentPhase = { type: "agentMessage", id: "a", text: "x", memoryCitation: null } satisfies ThreadItem;
// @ts-expect-error reasoning arrays reject null elements.
export const rejectNullReasoningEntry = { type: "reasoning", id: "r", summary: [null], content: [] } satisfies ThreadItem;
// @ts-expect-error command sources are closed.
export const rejectUnknownCommandSource = { type: "commandExecution", id: "c", command: "pwd", cwd: "/w", processId: null, source: "other", status: "completed", commandActions: [], aggregatedOutput: null, exitCode: null, durationMs: null } satisfies ThreadItem;
// @ts-expect-error file-change status is closed.
export const rejectUnknownPatchStatus = { type: "fileChange", id: "f", changes: [], status: "other" } satisfies ThreadItem;
// @ts-expect-error mcpAppResourceUri is non-null when present.
export const rejectNullMcpResource = { type: "mcpToolCall", id: "m", server: "s", tool: "t", status: "completed", arguments: null, appContext: null, mcpAppResourceUri: null, pluginId: null, result: null, error: null, durationMs: null } satisfies ThreadItem;
// @ts-expect-error dynamic contentItems is required nullable, not optional.
export const rejectMissingDynamicContent = { type: "dynamicToolCall", id: "d", namespace: null, tool: "t", arguments: null, status: "completed", success: null, durationMs: null } satisfies ThreadItem;
// @ts-expect-error optional map entries cannot be null.
export const rejectNullAgentState = { type: "collabAgentToolCall", id: "c", tool: "wait", status: "completed", senderThreadId: "s", receiverThreadIds: [], prompt: null, model: null, reasoningEffort: null, agentsStates: { a: null } } satisfies ThreadItem;
// @ts-expect-error subagent activity kinds are closed.
export const rejectUnknownActivity = { type: "subAgentActivity", id: "s", kind: "other", agentThreadId: "t", agentPath: "a" } satisfies ThreadItem;
// @ts-expect-error web-search action is required nullable, not optional.
export const rejectMissingWebAction = { type: "webSearch", id: "w", query: "q" } satisfies ThreadItem;
// @ts-expect-error sleep duration is non-null.
export const rejectNullSleepDuration = { type: "sleep", id: "s", durationMs: null } satisfies ThreadItem;
// @ts-expect-error image revisedPrompt is required nullable, not optional.
export const rejectMissingRevisedPrompt = { type: "imageGeneration", id: "i", status: "completed", result: "x" } satisfies ThreadItem;
// @ts-expect-error closed variants reject crossed fields.
export const rejectCrossedField = { type: "contextCompaction", id: "c", review: "crossed" } satisfies ThreadItem;

declare const threadItemIsExact: ThreadItemIsExact;
void threadItemIsExact;
