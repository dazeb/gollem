import type {
  Thread,
  ThreadStartedNotification,
  ThreadStatus,
  ThreadStatusChangedNotification,
  Turn,
  TurnCompletedNotification,
  TurnStartedNotification,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type Contracts = [
  Expect<Equal<ThreadStartedNotification, { thread: Thread }>>,
  Expect<Equal<ThreadStatusChangedNotification, { threadId: string; status: ThreadStatus }>>,
  Expect<Equal<TurnStartedNotification, { threadId: string; turn: Turn }>>,
  Expect<Equal<TurnCompletedNotification, { threadId: string; turn: Turn }>>,
];

declare const thread: Thread;
declare const turn: Turn;
export const notifications = [
  { thread } satisfies ThreadStartedNotification,
  { threadId: "", status: { type: "idle" } } satisfies ThreadStatusChangedNotification,
  { threadId: "", turn } satisfies TurnStartedNotification,
  { threadId: "thread-1", turn } satisfies TurnCompletedNotification,
];

// @ts-expect-error thread is required
export const missingThread: ThreadStartedNotification = {};
// @ts-expect-error status is required
export const missingStatus: ThreadStatusChangedNotification = { threadId: "" };
// @ts-expect-error nested status remains strict
export const badStatus: ThreadStatusChangedNotification = { threadId: "", status: { type: "active" } };
// @ts-expect-error turn is required
export const missingTurn: TurnStartedNotification = { threadId: "" };
// @ts-expect-error lifecycle records are closed
export const crossed: TurnCompletedNotification = { threadId: "", turn, at: "crossed" };

void (null as unknown as Contracts);
