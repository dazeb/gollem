import type {
  ByteRange,
  HookPromptFragment,
  ImageDetail,
  MemoryCitation,
  MemoryCitationEntry,
  MessagePhase,
  TextElement,
  UserInput,
} from "../gollem_appserver_protocol";

export const byteRange = { start: 9, end: 2 } satisfies ByteRange;
export const textElement = { byteRange, placeholder: null } satisfies TextElement;
export const imageDetail = "original" satisfies ImageDetail;
export const messagePhase = "final_answer" satisfies MessagePhase;
export const memoryEntry = {
  path: "relative.md",
  lineStart: 9,
  lineEnd: 2,
  note: "context",
} satisfies MemoryCitationEntry;
export const memoryCitation = {
  entries: [memoryEntry],
  threadIds: ["thread-1"],
} satisfies MemoryCitation;
export const hookPrompt = { text: "review", hookRunId: "hook-1" } satisfies HookPromptFragment;
export const inputs = [
  { type: "text", text: "hello", text_elements: [textElement] },
  { type: "image", url: "image.png" },
  { type: "image", detail: "high", url: "image.png" },
  { type: "localImage", path: "relative.png" },
  { type: "skill", name: "review", path: "skills/review/SKILL.md" },
  { type: "mention", name: "guide", path: "docs/guide.md" },
] satisfies UserInput[];

// @ts-expect-error placeholder is required nullable, not optional.
export const rejectMissingPlaceholder = { byteRange } satisfies TextElement;
// @ts-expect-error image detail is a closed public enum.
export const rejectImageDetail = "medium" satisfies ImageDetail;
// @ts-expect-error message phase uses the public snake_case literal.
export const rejectMessagePhase = "finalAnswer" satisfies MessagePhase;
// @ts-expect-error citation arrays are required.
export const rejectMissingCitationThreads = { entries: [memoryEntry] } satisfies MemoryCitation;
// @ts-expect-error hook fields use camelCase.
export const rejectHookSnakeCase = { text: "review", hook_run_id: "hook-1" } satisfies HookPromptFragment;
// @ts-expect-error text input requires the public snake_case text_elements field.
export const rejectMissingTextElements = { type: "text", text: "hello" } satisfies UserInput;
// @ts-expect-error explicit null is not valid for optional image detail.
export const rejectNullDetail = { type: "image", detail: null, url: "image.png" } satisfies UserInput;
// @ts-expect-error user-input variants cannot cross fields.
export const rejectCrossedInput = { type: "image", url: "image.png", path: "local.png" } satisfies UserInput;
