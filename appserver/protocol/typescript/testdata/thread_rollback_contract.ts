import type {
  MethodParamsByName,
  MethodResultsByName,
  Thread,
  ThreadRollbackParams,
  ThreadRollbackResponse,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2) ? true : false;
type Expect<T extends true> = T;

type Contracts = [
  Expect<Equal<ThreadRollbackParams, {
    numTurns: number;
    threadId: string;
  }>>,
  Expect<Equal<ThreadRollbackResponse, { thread: Thread }>>,
  Expect<Equal<"thread/rollback" extends keyof MethodParamsByName ? true : false, true>>,
  Expect<Equal<"thread/rollback" extends keyof MethodResultsByName ? true : false, true>>,
  Expect<Equal<MethodParamsByName["thread/rollback"], import("../gollem_appserver_protocol").ThreadHistoryRollbackParams>>,
  Expect<Equal<MethodResultsByName["thread/rollback"], import("../gollem_appserver_protocol").ThreadHistoryRollbackResult>>,
];

({ threadId: "thread", numTurns: 0 }) satisfies ThreadRollbackParams;
({ thread: null as unknown as Thread }) satisfies ThreadRollbackResponse;

// @ts-expect-error threadId is required.
({ numTurns: 0 }) satisfies ThreadRollbackParams;
// @ts-expect-error numTurns is required.
({ threadId: "thread" }) satisfies ThreadRollbackParams;
// @ts-expect-error response thread is required.
({}) satisfies ThreadRollbackResponse;

void (null as unknown as Contracts);
