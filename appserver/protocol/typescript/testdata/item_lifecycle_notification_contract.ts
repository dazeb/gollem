import type {
  BoundNotification,
  CommandExecutionItemCompletedNotificationParams,
  CommandExecutionItemStartedNotificationParams,
  DynamicToolCallItemCompletedNotificationParams,
  DynamicToolCallItemStartedNotificationParams,
  FileChangeItemCompletedNotificationParams,
  FileChangeItemStartedNotificationParams,
  ItemCompletedNotification,
  ItemLifecycleNotificationParams,
  ItemStartedNotification,
  MCPToolCallItemCompletedNotificationParams,
  MCPToolCallItemStartedNotificationParams,
  ThreadItem,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type ExpectedStarted = {
  item: ThreadItem;
  threadId: string;
  turnId: string;
  startedAtMs: number;
};
type ExpectedCompleted = {
  item: ThreadItem;
  threadId: string;
  turnId: string;
  completedAtMs: number;
};

type StartedIsExact = Expect<Equal<ItemStartedNotification, ExpectedStarted>>;
type CompletedIsExact = Expect<Equal<ItemCompletedNotification, ExpectedCompleted>>;

type ExpectedStartedBinding =
  | ItemStartedNotification
  | ItemLifecycleNotificationParams
  | DynamicToolCallItemStartedNotificationParams
  | CommandExecutionItemStartedNotificationParams
  | FileChangeItemStartedNotificationParams
  | MCPToolCallItemStartedNotificationParams;
type ExpectedCompletedBinding =
  | ItemCompletedNotification
  | ItemLifecycleNotificationParams
  | DynamicToolCallItemCompletedNotificationParams
  | CommandExecutionItemCompletedNotificationParams
  | FileChangeItemCompletedNotificationParams
  | MCPToolCallItemCompletedNotificationParams;

type StartedBindingIsExact = Expect<Equal<BoundNotification<"item/started">["params"], ExpectedStartedBinding>>;
type CompletedBindingIsExact = Expect<Equal<BoundNotification<"item/completed">["params"], ExpectedCompletedBinding>>;

export const exactStarted = {
  method: "item/started",
  params: {
    item: { type: "contextCompaction", id: "item-1" },
    threadId: "thread-1",
    turnId: "turn-1",
    startedAtMs: -1,
  },
} satisfies BoundNotification<"item/started">;
export const exactStartedParams = exactStarted.params satisfies ItemStartedNotification;

export const exactCompleted = {
  method: "item/completed",
  params: {
    item: { type: "plan", id: "item-2", text: "done" },
    threadId: "thread-1",
    turnId: "turn-1",
    completedAtMs: 2,
  },
} satisfies BoundNotification<"item/completed">;
export const exactCompletedParams = exactCompleted.params satisfies ItemCompletedNotification;

// @ts-expect-error item is required.
export const rejectStartedWithoutItem = { threadId: "thread-1", turnId: "turn-1", startedAtMs: 1 } satisfies ItemStartedNotification;
// @ts-expect-error threadId is required.
export const rejectStartedWithoutThread = { item: { type: "contextCompaction", id: "item-1" }, turnId: "turn-1", startedAtMs: 1 } satisfies ItemStartedNotification;
// @ts-expect-error turnId is required.
export const rejectStartedWithoutTurn = { item: { type: "contextCompaction", id: "item-1" }, threadId: "thread-1", startedAtMs: 1 } satisfies ItemStartedNotification;
// @ts-expect-error startedAtMs is required.
export const rejectStartedWithoutTimestamp = { item: { type: "contextCompaction", id: "item-1" }, threadId: "thread-1", turnId: "turn-1" } satisfies ItemStartedNotification;
// @ts-expect-error startedAtMs is numeric.
export const rejectStartedStringTimestamp = { item: { type: "contextCompaction", id: "item-1" }, threadId: "thread-1", turnId: "turn-1", startedAtMs: "1" } satisfies ItemStartedNotification;
// @ts-expect-error started notifications do not accept completedAtMs.
export const rejectStartedCompletedTimestamp = { item: { type: "contextCompaction", id: "item-1" }, threadId: "thread-1", turnId: "turn-1", startedAtMs: 1, completedAtMs: 1 } satisfies ItemStartedNotification;
// @ts-expect-error completedAtMs is required.
export const rejectCompletedWithoutTimestamp = { item: { type: "contextCompaction", id: "item-1" }, threadId: "thread-1", turnId: "turn-1" } satisfies ItemCompletedNotification;
// @ts-expect-error completed notifications do not accept startedAtMs.
export const rejectCompletedStartedTimestamp = { item: { type: "contextCompaction", id: "item-1" }, threadId: "thread-1", turnId: "turn-1", completedAtMs: 1, startedAtMs: 1 } satisfies ItemCompletedNotification;

void (0 as unknown as StartedIsExact);
void (0 as unknown as CompletedIsExact);
void (0 as unknown as StartedBindingIsExact);
void (0 as unknown as CompletedBindingIsExact);
