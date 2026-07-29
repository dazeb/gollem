import type {
  BoundNotification,
  BoundRequest,
  BoundResponse,
  MethodParamsByName,
  MethodResultsByName,
  ModelCatalogListParams,
  ModelCatalogListResponse,
  ParamsFor,
  ProviderListParams,
  ProviderListResponse,
  ResultFor,
  RuntimeDeltaNotification,
  RuntimeThreadNotification,
  RuntimeTurnNotification,
  ThreadRecord,
  ThreadRunStartParams,
  ThreadRunStartResult,
  ThreadStartParams,
  TurnRecord,
  TurnRunInterruptParams,
  TurnRunInterruptResult,
  TurnRunStartParams,
  TurnRunStartResult,
  TurnStartParams,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type Contracts = [
  Expect<Equal<ParamsFor<"provider/list">, ProviderListParams>>,
  Expect<Equal<ResultFor<"provider/list">, ProviderListResponse>>,
  Expect<Equal<ParamsFor<"model/list">, ModelCatalogListParams>>,
  Expect<Equal<ResultFor<"model/list">, ModelCatalogListResponse>>,
  Expect<Equal<ParamsFor<"thread/start">, ThreadRunStartParams>>,
  Expect<Equal<ResultFor<"thread/start">, ThreadRunStartResult>>,
  Expect<Equal<ParamsFor<"turn/start">, TurnRunStartParams>>,
  Expect<Equal<ResultFor<"turn/start">, TurnRunStartResult>>,
  Expect<Equal<ParamsFor<"turn/interrupt">, TurnRunInterruptParams>>,
  Expect<Equal<ResultFor<"turn/interrupt">, TurnRunInterruptResult>>,
  Expect<Equal<MethodParamsByName["thread/started"], RuntimeThreadNotification>>,
  Expect<Equal<MethodParamsByName["turn/started"], RuntimeTurnNotification>>,
  Expect<Equal<MethodParamsByName["turn/completed"], RuntimeTurnNotification>>,
  Expect<Equal<MethodParamsByName["item/agentMessage/delta"], RuntimeDeltaNotification>>,
  Expect<Equal<MethodParamsByName["item/reasoning/textDelta"], RuntimeDeltaNotification>>,
  Expect<Equal<Equal<ThreadRunStartParams, ThreadStartParams>, false>>,
  Expect<Equal<Equal<TurnRunStartParams, TurnStartParams>, false>>,
  Expect<Equal<keyof MethodResultsByName & "thread/started", never>>,
];

declare const thread: ThreadRecord;
declare const turn: TurnRecord;

export const providerRequest = {
  id: 1,
  method: "provider/list",
  params: { configuredOnly: true },
} satisfies BoundRequest<"provider/list">;

export const modelRequest = {
  id: 2,
  method: "model/list",
  params: { providerId: "openai", limit: 25 },
} satisfies BoundRequest<"model/list">;

export const threadStartRequest = {
  id: 3,
  method: "thread/start",
  params: {
    title: "Typed run",
    workspace: "/workspace",
    prompt: "Inspect the repository",
    providerId: "openai",
    model: "gpt-5",
  },
} satisfies BoundRequest<"thread/start">;

export const turnStartRequest = {
  id: 4,
  method: "turn/start",
  params: {
    threadId: "thread-1",
    prompt: "Continue",
    providerId: "openai",
    model: "gpt-5",
  },
} satisfies BoundRequest<"turn/start">;

export const interruptRequest = {
  id: 5,
  method: "turn/interrupt",
  params: { threadId: "thread-1", turnId: "turn-1" },
} satisfies BoundRequest<"turn/interrupt">;

export const threadStartResponse = {
  id: 3,
  result: { thread, turn },
} satisfies BoundResponse<"thread/start">;

export const turnStartResponse = {
  id: 4,
  result: { thread, turn },
} satisfies BoundResponse<"turn/start">;

export const interruptResponse = {
  id: 5,
  result: { ok: true, turnId: "turn-1", turn },
} satisfies BoundResponse<"turn/interrupt">;

export const threadStarted = {
  method: "thread/started",
  params: {
    threadId: "thread-1",
    status: "active",
    thread,
    at: "2026-07-29T00:00:00Z",
  },
} satisfies BoundNotification<"thread/started">;

export const turnStarted = {
  method: "turn/started",
  params: {
    threadId: "thread-1",
    turnId: "turn-1",
    status: "running",
    turn,
    at: "2026-07-29T00:00:00Z",
  },
} satisfies BoundNotification<"turn/started">;

export const messageDelta = {
  method: "item/agentMessage/delta",
  params: {
    threadId: "thread-1",
    turnId: "turn-1",
    delta: "hello",
    at: "2026-07-29T00:00:00Z",
  },
} satisfies BoundNotification<"item/agentMessage/delta">;

// @ts-expect-error runtime thread/start does not accept credential material.
export const rejectCredential = { id: 6, method: "thread/start", params: { prompt: "hello", apiKey: "secret" } } satisfies BoundRequest<"thread/start">;

// @ts-expect-error runtime turn/start excludes the public request's sandbox policy.
export const rejectPublicTurnStart = { id: 7, method: "turn/start", params: { threadId: "thread-1", sandboxPolicy: { type: "dangerFullAccess" } } } satisfies BoundRequest<"turn/start">;

void (null as unknown as Contracts);
