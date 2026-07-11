import type {
  DynamicToolCallOutputContentItem,
  DynamicToolCallParams,
  DynamicToolCallResponse,
  ResultFor,
} from "../gollem_appserver_protocol";

export const params = {
  threadId: "thread-1",
  turnId: "turn-1",
  callId: "call-1",
  namespace: null,
  tool: "client.search",
  arguments: { query: "gollem" },
  requestId: "interaction-1",
  toolName: "client.search",
} satisfies DynamicToolCallParams;

export const text = { type: "inputText", text: "match" } satisfies DynamicToolCallOutputContentItem;
export const image = {
  type: "inputImage",
  imageUrl: "data:image/png;base64,AA==",
} satisfies DynamicToolCallOutputContentItem;
export const response = { contentItems: [text, image], success: true } satisfies DynamicToolCallResponse;
response satisfies ResultFor<"item/tool/call">;

// @ts-expect-error callId is required.
export const rejectMissingCall = { ...params, callId: undefined } satisfies DynamicToolCallParams;
// @ts-expect-error namespace is required and explicitly nullable.
export const rejectMissingNamespace = { ...params, namespace: undefined } satisfies DynamicToolCallParams;
// @ts-expect-error text content requires text.
export const rejectMissingText = { type: "inputText" } satisfies DynamicToolCallOutputContentItem;
// @ts-expect-error image content cannot carry text.
export const rejectCrossVariant = { type: "inputImage", imageUrl: "image", text: "bad" } satisfies DynamicToolCallOutputContentItem;
// @ts-expect-error content variants are closed.
export const rejectUnknownVariant = { type: "video", url: "bad" } satisfies DynamicToolCallOutputContentItem;
// @ts-expect-error success is required.
export const rejectMissingSuccess = { contentItems: [] } satisfies DynamicToolCallResponse;
