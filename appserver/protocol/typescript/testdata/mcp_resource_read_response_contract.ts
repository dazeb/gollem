import type {
  McpResourceReadResponse,
  MethodParamsByName,
  MethodResultsByName,
  ResourceContent,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type Expected = { contents: Array<ResourceContent> };
type Contracts = [
  Expect<Equal<McpResourceReadResponse, Expected>>,
  Expect<Equal<"mcpServer/resource/read" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"mcpServer/resource/read" extends keyof MethodResultsByName ? true : false, false>>,
];

export const empty = { contents: [] } satisfies McpResourceReadResponse;
export const text = {
  contents: [{ uri: "resource://text", text: "hello" }],
} satisfies McpResourceReadResponse;
export const mixed = {
  contents: [
    { uri: "resource://text", mimeType: "text/plain", text: "hello" },
    { uri: "resource://blob", blob: "AA==", _meta: { source: ["client", null] } },
  ],
} satisfies McpResourceReadResponse;
export const crossed = {
  contents: [{ uri: "resource://crossed", text: "text", blob: "blob" }],
} satisfies McpResourceReadResponse;

// @ts-expect-error contents is required.
export const rejectMissingContents = {} satisfies McpResourceReadResponse;
// @ts-expect-error contents is a non-null array.
export const rejectNullContents = { contents: null } satisfies McpResourceReadResponse;
// @ts-expect-error contents is an array.
export const rejectObjectContents = { contents: {} } satisfies McpResourceReadResponse;
// @ts-expect-error contents is an array.
export const rejectStringContents = { contents: "value" } satisfies McpResourceReadResponse;
// @ts-expect-error contents cannot be explicitly undefined.
export const rejectUndefinedContents = { contents: undefined } satisfies McpResourceReadResponse;
// @ts-expect-error array elements must be resource-content values.
export const rejectNullElement = { contents: [null] } satisfies McpResourceReadResponse;
// @ts-expect-error array elements require uri.
export const rejectMissingElementURI = { contents: [{ text: "value" }] } satisfies McpResourceReadResponse;
// @ts-expect-error array elements require text or blob.
export const rejectMissingElementContent = { contents: [{ uri: "resource://missing" }] } satisfies McpResourceReadResponse;
// @ts-expect-error exact plural wire name is required.
export const rejectContentAlias = { content: [] } satisfies McpResourceReadResponse;
// @ts-expect-error fields absent from the public response are rejected.
export const rejectExtra = { contents: [], extra: true } satisfies McpResourceReadResponse;

void (null as unknown as Contracts);
