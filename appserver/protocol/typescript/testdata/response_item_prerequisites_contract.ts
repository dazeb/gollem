import type {
  ContentItem,
  FunctionCallOutputBody,
  FunctionCallOutputContentItem,
  InternalChatMessageMetadataPassthrough,
  LocalShellAction,
  LocalShellStatus,
} from "../gollem_appserver_protocol";

export const contentItems = [
  { type: "input_text", text: "hello" },
  { type: "input_image", image_url: "image.png" },
  { type: "input_image", image_url: "image.png", detail: "original" },
  { type: "output_text", text: "done" },
] satisfies ContentItem[];

export const functionOutputItems = [
  { type: "input_text", text: "tool output" },
  { type: "input_image", image_url: "image.png", detail: "low" },
  { type: "encrypted_content", encrypted_content: "cipher" },
] satisfies FunctionCallOutputContentItem[];

export const functionOutputBodies = [
  "plain",
  [],
  [{ type: "input_text", text: "tool output" }],
] satisfies FunctionCallOutputBody[];

export const metadata = [
  {},
  { turn_id: "turn-1" },
] satisfies InternalChatMessageMetadataPassthrough[];

export const shellStatuses = ["completed", "in_progress", "incomplete"] satisfies LocalShellStatus[];
export const shellActions = [
  {
    type: "exec",
    command: [],
    timeout_ms: null,
    working_directory: null,
    env: null,
    user: null,
  },
  {
    type: "exec",
    command: ["go", "test", "./..."],
    timeout_ms: 60_000,
    working_directory: "/workspace",
    env: { CI: "1" },
    user: "runner",
  },
] satisfies LocalShellAction[];

// @ts-expect-error image detail is non-null when present.
export const rejectNullDetail = { type: "input_image", image_url: "image.png", detail: null } satisfies ContentItem;
// @ts-expect-error content variants cannot cross fields.
export const rejectCrossedContent = { type: "output_text", text: "done", image_url: "image.png" } satisfies ContentItem;
// @ts-expect-error encrypted content uses snake_case.
export const rejectCamelEncrypted = { type: "encrypted_content", encryptedContent: "cipher" } satisfies FunctionCallOutputContentItem;
// @ts-expect-error function output bodies contain strict content items.
export const rejectOutputTextBody = [{ type: "output_text", text: "unsupported" }] satisfies FunctionCallOutputBody;
// @ts-expect-error metadata uses snake_case.
export const rejectCamelTurn = { turnId: "turn-1" } satisfies InternalChatMessageMetadataPassthrough;
// @ts-expect-error optional metadata is non-null when present.
export const rejectNullTurn = { turn_id: null } satisfies InternalChatMessageMetadataPassthrough;
// @ts-expect-error LocalShellStatus is closed and snake_case.
export const rejectShellStatus = "inProgress" satisfies LocalShellStatus;
// @ts-expect-error local-shell nullable fields are required.
export const rejectMissingShellFields = { type: "exec", command: [] } satisfies LocalShellAction;
export const rejectNumericEnvironment = {
  type: "exec",
  command: [],
  timeout_ms: null,
  working_directory: null,
  // @ts-expect-error shell environment values are strings.
  env: { CI: 1 },
  user: null,
} satisfies LocalShellAction;
