import type {
  JsonValue,
  McpServerToolCallResponse,
  MethodParamsByName,
  MethodResultsByName,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type Expected = {
  content: Array<JsonValue>;
  structuredContent?: JsonValue;
  isError?: boolean;
  _meta?: JsonValue;
};
type Contracts = [
  Expect<Equal<McpServerToolCallResponse, Expected>>,
  Expect<
    Equal<
      "mcpServer/tool/call" extends keyof MethodParamsByName ? true : false,
      false
    >
  >,
  Expect<
    Equal<
      "mcpServer/tool/call" extends keyof MethodResultsByName ? true : false,
      false
    >
  >,
];

export const emptyContent = { content: [] } satisfies McpServerToolCallResponse;
export const arbitraryJSON = {
  content: [null, "text", true, 9007199254740991, { nested: [1, false] }],
  structuredContent: { result: ["ok", null] },
  isError: false,
  _meta: ["source", { client: true }],
} satisfies McpServerToolCallResponse;
export const nullJSONOptions = {
  content: [],
  structuredContent: null,
  _meta: null,
} satisfies McpServerToolCallResponse;

// @ts-expect-error content is required.
export const rejectMissingContent = {} satisfies McpServerToolCallResponse;
// @ts-expect-error content is a non-null array.
export const rejectNullContent = { content: null } satisfies McpServerToolCallResponse;
// @ts-expect-error content is an array.
export const rejectObjectContent = { content: {} } satisfies McpServerToolCallResponse;
// @ts-expect-error content cannot be explicitly undefined.
export const rejectUndefinedContent = { content: undefined } satisfies McpServerToolCallResponse;
// @ts-expect-error content elements must be JSON values.
export const rejectUndefinedElement = { content: [undefined] } satisfies McpServerToolCallResponse;
// @ts-expect-error bigint is not a JSON value.
export const rejectBigIntElement = { content: [1n] } satisfies McpServerToolCallResponse;
// @ts-expect-error functions are not JSON values.
export const rejectFunctionElement = { content: [() => true] } satisfies McpServerToolCallResponse;
// @ts-expect-error isError is optional boolean, not null.
export const rejectNullError = { content: [], isError: null } satisfies McpServerToolCallResponse;
// @ts-expect-error isError is a boolean.
export const rejectStringError = { content: [], isError: "false" } satisfies McpServerToolCallResponse;
// @ts-expect-error exact lower-camel wire names are required.
export const rejectStructuredAlias = { content: [], structured_content: null } satisfies McpServerToolCallResponse;
// @ts-expect-error exact lower-camel wire names are required.
export const rejectErrorAlias = { content: [], is_error: false } satisfies McpServerToolCallResponse;
// @ts-expect-error the metadata wire name includes the underscore.
export const rejectMetaAlias = { content: [], meta: null } satisfies McpServerToolCallResponse;
// @ts-expect-error live response identity fields are not part of the public contract.
export const rejectLiveIdentity = { content: [], serverName: "repo" } satisfies McpServerToolCallResponse;
// @ts-expect-error live nested result is not part of the public contract.
export const rejectLiveResult = { content: [], result: {} } satisfies McpServerToolCallResponse;
// @ts-expect-error live text is not part of the public contract.
export const rejectLiveText = { content: [], text: "pong" } satisfies McpServerToolCallResponse;
// @ts-expect-error optional structured content may be omitted or JSON null, not explicitly undefined.
export const rejectUndefinedStructuredContent = { content: [], structuredContent: undefined } satisfies McpServerToolCallResponse;
// @ts-expect-error optional error may be omitted, not explicitly undefined.
export const rejectUndefinedError = { content: [], isError: undefined } satisfies McpServerToolCallResponse;
// @ts-expect-error optional metadata may be omitted or JSON null, not explicitly undefined.
export const rejectUndefinedMeta = { content: [], _meta: undefined } satisfies McpServerToolCallResponse;

void (null as unknown as Contracts);
