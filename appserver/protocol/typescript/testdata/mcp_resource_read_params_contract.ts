import type {
  McpResourceReadParams,
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
  threadId?: string | null;
  server: string;
  uri: string;
};
type Contracts = [
  Expect<Equal<McpResourceReadParams, Expected>>,
  Expect<
    Equal<
      "mcpServer/resource/read" extends keyof MethodParamsByName ? true : false,
      false
    >
  >,
  Expect<
    Equal<
      "mcpServer/resource/read" extends keyof MethodResultsByName ? true : false,
      false
    >
  >,
];

export const emptyStrings = {
  server: "",
  uri: "",
} satisfies McpResourceReadParams;
export const nullableThread = {
  threadId: null,
  server: "repo",
  uri: "file:///workspace/README.md",
} satisfies McpResourceReadParams;
export const populated = {
  threadId: "thread-1",
  server: "repo",
  uri: "file:///workspace/README.md",
} satisfies McpResourceReadParams;

// @ts-expect-error server is required.
export const rejectMissingServer = { uri: "resource" } satisfies McpResourceReadParams;
// @ts-expect-error URI is required.
export const rejectMissingURI = { server: "repo" } satisfies McpResourceReadParams;
// @ts-expect-error server is a non-null string.
export const rejectNullServer = { server: null, uri: "resource" } satisfies McpResourceReadParams;
// @ts-expect-error server is a string.
export const rejectNumericServer = { server: 1, uri: "resource" } satisfies McpResourceReadParams;
// @ts-expect-error URI is a non-null string.
export const rejectNullURI = { server: "repo", uri: null } satisfies McpResourceReadParams;
// @ts-expect-error URI is a string.
export const rejectNumericURI = { server: "repo", uri: 1 } satisfies McpResourceReadParams;
// @ts-expect-error thread ids are nullable strings only.
export const rejectNumericThread = { threadId: 1, server: "repo", uri: "resource" } satisfies McpResourceReadParams;
// @ts-expect-error exact lower-camel wire names are required.
export const rejectSnakeThread = { thread_id: "thread-1", server: "repo", uri: "resource" } satisfies McpResourceReadParams;
// @ts-expect-error live server aliases are not part of the public contract.
export const rejectLiveServerAlias = { serverName: "repo", server: "repo", uri: "resource" } satisfies McpResourceReadParams;
// @ts-expect-error live resource aliases are not part of the public contract.
export const rejectLiveResourceAlias = { resourceUri: "resource", server: "repo", uri: "resource" } satisfies McpResourceReadParams;
// @ts-expect-error exact optional properties may be omitted or null, not explicitly undefined.
export const rejectUndefinedThread = { threadId: undefined, server: "repo", uri: "resource" } satisfies McpResourceReadParams;

void (null as unknown as Contracts);
