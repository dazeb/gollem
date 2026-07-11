import type { ResponseItem } from "../gollem_appserver_protocol";

export const responseItems = [
  { type: "message", role: "assistant", content: [] },
  {
    type: "message",
    id: "msg-1",
    role: "assistant",
    content: [{ type: "output_text", text: "done" }],
    phase: "final_answer",
    internal_chat_message_metadata_passthrough: { turn_id: "turn-1" },
  },
  { type: "agent_message", author: "agent-a", recipient: "agent-b", content: [] },
  {
    type: "agent_message",
    content: [{ type: "input_text", text: "hello" }],
    author: "agent-a",
    recipient: "agent-b",
  },
  { type: "reasoning", summary: [], encrypted_content: null },
  {
    type: "reasoning",
    summary: [{ type: "summary_text", text: "summary" }],
    content: [{ type: "reasoning_text", text: "thinking" }],
    encrypted_content: "cipher",
  },
  {
    type: "local_shell_call",
    call_id: null,
    status: "in_progress",
    action: {
      type: "exec",
      command: [],
      timeout_ms: null,
      working_directory: null,
      env: null,
      user: null,
    },
  },
  { type: "function_call", name: "run", arguments: "{}", call_id: "call-1" },
  { type: "tool_search_call", call_id: null, execution: "search", arguments: null },
  { type: "tool_search_call", call_id: "call-1", status: "completed", execution: "search", arguments: { query: "go" } },
  { type: "function_call_output", call_id: "call-1", output: "done" },
  { type: "function_call_output", call_id: "call-1", output: [{ type: "input_text", text: "done" }] },
  { type: "custom_tool_call", call_id: "call-1", name: "shell", input: "pwd" },
  { type: "custom_tool_call_output", call_id: "call-1", name: "shell", output: [] },
  { type: "tool_search_output", call_id: null, status: "completed", execution: "search", tools: [null, 1, "tool", { name: "run" }] },
  { type: "web_search_call" },
  { type: "web_search_call", status: "completed", action: { type: "openPage", url: null } },
  { type: "image_generation_call", status: "completed", revised_prompt: "draw", result: "base64" },
  { type: "compaction", encrypted_content: "cipher" },
  { type: "compaction_trigger" },
  { type: "context_compaction" },
  { type: "context_compaction", encrypted_content: "cipher" },
  { type: "other" },
] satisfies ResponseItem[];

// @ts-expect-error message content is required.
export const rejectMissingMessageContent = { type: "message", role: "assistant" } satisfies ResponseItem;
// @ts-expect-error optional ids are non-null when present.
export const rejectNullResponseItemId = { type: "message", id: null, role: "assistant", content: [] } satisfies ResponseItem;
// @ts-expect-error optional metadata is non-null when present.
export const rejectNullResponseMetadata = { type: "message", role: "assistant", content: [], internal_chat_message_metadata_passthrough: null } satisfies ResponseItem;
// @ts-expect-error optional reasoning content is non-null when present.
export const rejectNullReasoningContent = { type: "reasoning", summary: [], content: null, encrypted_content: null } satisfies ResponseItem;
// @ts-expect-error reasoning encrypted content is required even when null.
export const rejectMissingReasoningEncryptedContent = { type: "reasoning", summary: [] } satisfies ResponseItem;
export const rejectMissingShellCallId = {
  type: "local_shell_call",
  status: "completed",
  action: { type: "exec", command: [], timeout_ms: null, working_directory: null, env: null, user: null },
// @ts-expect-error local shell call_id is required even when null.
} satisfies ResponseItem;
// @ts-expect-error function namespaces are non-null when present.
export const rejectNullFunctionNamespace = { type: "function_call", name: "run", namespace: null, arguments: "{}", call_id: "call-1" } satisfies ResponseItem;
// @ts-expect-error tool-search call_id is required even when null.
export const rejectMissingToolSearchCallId = { type: "tool_search_call", execution: "search", arguments: {} } satisfies ResponseItem;
// @ts-expect-error function output uses the strict output-body union.
export const rejectObjectFunctionOutput = { type: "function_call_output", call_id: "call-1", output: {} } satisfies ResponseItem;
// @ts-expect-error tools must be an array.
export const rejectObjectTools = { type: "tool_search_output", call_id: null, status: "completed", execution: "search", tools: {} } satisfies ResponseItem;
// @ts-expect-error web-search actions use the established app-server action contract.
export const rejectSnakeCaseWebAction = { type: "web_search_call", action: { type: "open_page", url: null } } satisfies ResponseItem;
// @ts-expect-error revised prompts are non-null when present.
export const rejectNullRevisedPrompt = { type: "image_generation_call", status: "completed", revised_prompt: null, result: "base64" } satisfies ResponseItem;
// @ts-expect-error compaction encrypted content is required and non-null.
export const rejectNullCompactionContent = { type: "compaction", encrypted_content: null } satisfies ResponseItem;
// @ts-expect-error context-compaction encrypted content is non-null when present.
export const rejectNullContextCompactionContent = { type: "context_compaction", encrypted_content: null } satisfies ResponseItem;
// @ts-expect-error closed variants reject crossed fields.
export const rejectCrossedResponseItem = { type: "other", call_id: "call-1" } satisfies ResponseItem;
