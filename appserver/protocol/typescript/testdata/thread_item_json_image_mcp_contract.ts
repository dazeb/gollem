import type {
  ImageGenerationItem,
  JsonValue,
  McpToolCallResult,
} from "../gollem_appserver_protocol";

export const jsonValues = [
  null,
  true,
  1,
  "text",
  [null, { nested: [1, false] }],
  { value: "ok" },
] satisfies JsonValue[];

export const imageItems = [
  { id: "image-1", status: "completed", revisedPrompt: null, result: "base64" },
  {
    id: "image-2",
    status: "completed",
    revisedPrompt: "draw",
    result: "base64",
    savedPath: "/workspace/image.png",
  },
] satisfies ImageGenerationItem[];

export const mcpResults = [
  { content: [], structuredContent: null, _meta: null },
  {
    content: [null, "text", 1, { ok: true }],
    structuredContent: { result: [1, 2] },
    _meta: ["source"],
  },
] satisfies McpToolCallResult[];

// @ts-expect-error undefined is not JSON.
undefined satisfies JsonValue;
// @ts-expect-error functions are not JSON.
(() => true) satisfies JsonValue;
// @ts-expect-error revisedPrompt is required nullable.
({ id: "image-1", status: "completed", result: "base64" }) satisfies ImageGenerationItem;
// @ts-expect-error optional savedPath is non-null when present.
({ id: "image-1", status: "completed", revisedPrompt: null, result: "base64", savedPath: null }) satisfies ImageGenerationItem;
// @ts-expect-error MCP content is required and non-null.
({ content: null, structuredContent: null, _meta: null }) satisfies McpToolCallResult;
// @ts-expect-error structuredContent is required nullable.
({ content: [], _meta: null }) satisfies McpToolCallResult;
// @ts-expect-error _meta is required nullable.
({ content: [], structuredContent: null }) satisfies McpToolCallResult;
