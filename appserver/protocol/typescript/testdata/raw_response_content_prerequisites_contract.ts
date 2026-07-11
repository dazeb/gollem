import type {
  AgentMessageInputContent,
  ReasoningItemContent,
  ReasoningItemReasoningSummary,
} from "../gollem_appserver_protocol";

export const agentMessageContent = [
  { type: "input_text", text: "hello" },
  { type: "encrypted_content", encrypted_content: "cipher" },
] satisfies AgentMessageInputContent[];
export const reasoningContent = [
  { type: "reasoning_text", text: "thinking" },
  { type: "text", text: "visible" },
] satisfies ReasoningItemContent[];
export const reasoningSummary = {
  type: "summary_text",
  text: "summary",
} satisfies ReasoningItemReasoningSummary;

// @ts-expect-error input_text requires text.
export const rejectMissingInputText = { type: "input_text" } satisfies AgentMessageInputContent;
// @ts-expect-error encrypted content uses snake_case.
export const rejectCamelEncryptedContent = { type: "encrypted_content", encryptedContent: "cipher" } satisfies AgentMessageInputContent;
// @ts-expect-error agent-message variants cannot cross fields.
export const rejectCrossedAgentContent = { type: "input_text", text: "hello", encrypted_content: "cipher" } satisfies AgentMessageInputContent;
// @ts-expect-error reasoning content discriminators are snake_case.
export const rejectCamelReasoningType = { type: "reasoningText", text: "thinking" } satisfies ReasoningItemContent;
// @ts-expect-error reasoning content requires text.
export const rejectMissingReasoningText = { type: "text" } satisfies ReasoningItemContent;
// @ts-expect-error reasoning summary discriminator is snake_case.
export const rejectCamelSummaryType = { type: "summaryText", text: "summary" } satisfies ReasoningItemReasoningSummary;
