import type {
  ListMcpServerStatusParams,
  McpServerStatusDetail,
  MethodParamsByName,
  MethodResultsByName,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? (<T>() => T extends B ? 1 : 2) extends
        (<T>() => T extends A ? 1 : 2)
      ? true
      : false
    : false;
type Expect<T extends true> = T;

type Expected = {
  cursor?: string | null;
  limit?: number | null;
  detail?: McpServerStatusDetail | null;
  threadId?: string | null;
};
type Contracts = [
  Expect<Equal<ListMcpServerStatusParams, Expected>>,
  Expect<
    Equal<
      "mcpServerStatus/list" extends keyof MethodParamsByName ? true : false,
      false
    >
  >,
  Expect<
    Equal<
      "mcpServerStatus/list" extends keyof MethodResultsByName ? true : false,
      false
    >
  >,
];

export const empty = {} satisfies ListMcpServerStatusParams;
export const nullable = {
  cursor: null,
  limit: null,
  detail: null,
  threadId: null,
} satisfies ListMcpServerStatusParams;
export const lowerBoundary = {
  cursor: "",
  limit: 0,
  detail: "full",
  threadId: "",
} satisfies ListMcpServerStatusParams;
export const upperBoundary = {
  cursor: "next",
  limit: 4294967295,
  detail: "toolsAndAuthOnly",
  threadId: "thread-1",
} satisfies ListMcpServerStatusParams;

// @ts-expect-error cursors are nullable strings only.
export const rejectNumericCursor = { cursor: 1 } satisfies ListMcpServerStatusParams;
// @ts-expect-error limits are nullable numbers only.
export const rejectStringLimit = { limit: "1" } satisfies ListMcpServerStatusParams;
// @ts-expect-error status detail values are closed.
export const rejectUnknownDetail = { detail: "other" } satisfies ListMcpServerStatusParams;
// @ts-expect-error thread ids are nullable strings only.
export const rejectNumericThread = { threadId: 1 } satisfies ListMcpServerStatusParams;
// @ts-expect-error exact lower-camel wire names are required.
export const rejectSnakeThread = { thread_id: "thread-1" } satisfies ListMcpServerStatusParams;
// @ts-expect-error live server filters are not part of the public contract.
export const rejectLiveFilter = { serverId: "server-1" } satisfies ListMcpServerStatusParams;
// @ts-expect-error exact optional properties may be omitted or null, not explicitly undefined.
export const rejectUndefinedDetail = { detail: undefined } satisfies ListMcpServerStatusParams;

void (null as unknown as Contracts);
