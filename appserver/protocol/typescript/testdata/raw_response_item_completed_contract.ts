import type {
  BoundNotification,
  RawResponseItemCompletedNotification,
  ResponseItem,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type ExpectedRawResponseItemCompletedNotification = {
  threadId: string;
  turnId: string;
  item: ResponseItem;
};
type RawResponseNotificationIsExact = Expect<
  Equal<RawResponseItemCompletedNotification, ExpectedRawResponseItemCompletedNotification>
>;

export const rawMessageCompleted = {
  threadId: "thread-1",
  turnId: "turn-1",
  item: {
    type: "message",
    role: "assistant",
    content: [{ type: "output_text", text: "done" }],
  },
} satisfies RawResponseItemCompletedNotification;

export const rawToolSearchCompleted = {
  threadId: "thread-1",
  turnId: "turn-1",
  item: {
    type: "tool_search_call",
    call_id: null,
    status: "completed",
    execution: "search",
    arguments: { nested: [1, true, null] },
  },
} satisfies RawResponseItemCompletedNotification;

// @ts-expect-error threadId is required.
({ turnId: "turn-1", item: { type: "other" } }) satisfies RawResponseItemCompletedNotification;
// @ts-expect-error turnId is required.
({ threadId: "thread-1", item: { type: "other" } }) satisfies RawResponseItemCompletedNotification;
// @ts-expect-error item is required.
({ threadId: "thread-1", turnId: "turn-1" }) satisfies RawResponseItemCompletedNotification;
// @ts-expect-error threadId is non-null.
({ threadId: null, turnId: "turn-1", item: { type: "other" } }) satisfies RawResponseItemCompletedNotification;
// @ts-expect-error nested response items remain strict.
({ threadId: "thread-1", turnId: "turn-1", item: { type: "other", id: "crossed" } }) satisfies RawResponseItemCompletedNotification;
// @ts-expect-error notification objects are closed.
({ threadId: "thread-1", turnId: "turn-1", item: { type: "other" }, extra: true }) satisfies RawResponseItemCompletedNotification;
// @ts-expect-error the blocked method has no generated bound payload.
export type RawResponseMethodRemainsUnbound = BoundNotification<"rawResponseItem/completed">;

declare const rawResponseNotificationIsExact: RawResponseNotificationIsExact;
void rawResponseNotificationIsExact;
