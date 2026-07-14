import type {
  JsonValue,
  McpServerToolCallParams,
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
  threadId: string;
  server: string;
  tool: string;
  arguments?: JsonValue;
  _meta?: JsonValue;
};
type Contracts = [
  Expect<Equal<McpServerToolCallParams, Expected>>,
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

export const emptyStrings = {
  threadId: "",
  server: "",
  tool: "",
} satisfies McpServerToolCallParams;
export const nullOptions = {
  threadId: "thread-1",
  server: "repo",
  tool: "echo",
  arguments: null,
  _meta: null,
} satisfies McpServerToolCallParams;
export const arbitraryJSON = {
  threadId: "thread-1",
  server: "repo",
  tool: "echo",
  arguments: {
    nested: [true, null, "value", 9007199254740991],
  },
  _meta: [1, { source: "client" }],
} satisfies McpServerToolCallParams;

// @ts-expect-error threadId is required.
export const rejectMissingThread = { server: "repo", tool: "echo" } satisfies McpServerToolCallParams;
// @ts-expect-error server is required.
export const rejectMissingServer = { threadId: "thread-1", tool: "echo" } satisfies McpServerToolCallParams;
// @ts-expect-error tool is required.
export const rejectMissingTool = { threadId: "thread-1", server: "repo" } satisfies McpServerToolCallParams;
// @ts-expect-error threadId is a non-null string.
export const rejectNullThread = { threadId: null, server: "repo", tool: "echo" } satisfies McpServerToolCallParams;
// @ts-expect-error threadId is a string.
export const rejectNumericThread = { threadId: 1, server: "repo", tool: "echo" } satisfies McpServerToolCallParams;
// @ts-expect-error server is a non-null string.
export const rejectNullServer = { threadId: "thread-1", server: null, tool: "echo" } satisfies McpServerToolCallParams;
// @ts-expect-error server is a string.
export const rejectNumericServer = { threadId: "thread-1", server: 1, tool: "echo" } satisfies McpServerToolCallParams;
// @ts-expect-error tool is a non-null string.
export const rejectNullTool = { threadId: "thread-1", server: "repo", tool: null } satisfies McpServerToolCallParams;
// @ts-expect-error tool is a string.
export const rejectNumericTool = { threadId: "thread-1", server: "repo", tool: 1 } satisfies McpServerToolCallParams;
// @ts-expect-error exact lower-camel wire names are required.
export const rejectSnakeThread = { thread_id: "thread-1", threadId: "thread-1", server: "repo", tool: "echo" } satisfies McpServerToolCallParams;
// @ts-expect-error live server aliases are not part of the public contract.
export const rejectLiveServerAlias = { serverName: "repo", threadId: "thread-1", server: "repo", tool: "echo" } satisfies McpServerToolCallParams;
// @ts-expect-error live tool aliases are not part of the public contract.
export const rejectLiveToolAlias = { toolName: "echo", threadId: "thread-1", server: "repo", tool: "echo" } satisfies McpServerToolCallParams;
// @ts-expect-error live argument aliases are not part of the public contract.
export const rejectLiveArgumentsAlias = { args: {}, threadId: "thread-1", server: "repo", tool: "echo" } satisfies McpServerToolCallParams;
// @ts-expect-error the wire metadata name includes the underscore.
export const rejectMetaAlias = { meta: {}, threadId: "thread-1", server: "repo", tool: "echo" } satisfies McpServerToolCallParams;
// @ts-expect-error optional arguments may be omitted or JSON null, not explicitly undefined.
export const rejectUndefinedArguments = { arguments: undefined, threadId: "thread-1", server: "repo", tool: "echo" } satisfies McpServerToolCallParams;
// @ts-expect-error optional metadata may be omitted or JSON null, not explicitly undefined.
export const rejectUndefinedMeta = { _meta: undefined, threadId: "thread-1", server: "repo", tool: "echo" } satisfies McpServerToolCallParams;

void (null as unknown as Contracts);
