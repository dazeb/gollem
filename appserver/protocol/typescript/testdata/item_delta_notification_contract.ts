import type {
  AgentMessageDeltaNotification,
  PlanDeltaNotification,
  ReasoningSummaryPartAddedNotification,
  ReasoningSummaryTextDeltaNotification,
  ReasoningTextDeltaNotification,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type Delta = { threadId: string; turnId: string; itemId: string; delta: string };
type Contracts = [
  Expect<Equal<AgentMessageDeltaNotification, Delta>>,
  Expect<Equal<PlanDeltaNotification, Delta>>,
  Expect<Equal<ReasoningSummaryPartAddedNotification, {
    threadId: string;
    turnId: string;
    itemId: string;
    summaryIndex: number;
  }>>,
  Expect<Equal<ReasoningSummaryTextDeltaNotification, {
    threadId: string;
    turnId: string;
    itemId: string;
    delta: string;
    summaryIndex: number;
  }>>,
  Expect<Equal<ReasoningTextDeltaNotification, {
    threadId: string;
    turnId: string;
    itemId: string;
    delta: string;
    contentIndex: number;
  }>>,
];

export const agent = { threadId: "", turnId: "", itemId: "", delta: "" } satisfies AgentMessageDeltaNotification;
export const plan = { threadId: "thread", turnId: "turn", itemId: "item", delta: "step" } satisfies PlanDeltaNotification;
export const summaryPart = { threadId: "thread", turnId: "turn", itemId: "item", summaryIndex: -1 } satisfies ReasoningSummaryPartAddedNotification;
export const summaryText = { threadId: "thread", turnId: "turn", itemId: "item", delta: "summary", summaryIndex: 0 } satisfies ReasoningSummaryTextDeltaNotification;
export const reasoningText = { threadId: "thread", turnId: "turn", itemId: "item", delta: "reason", contentIndex: 1 } satisfies ReasoningTextDeltaNotification;

// @ts-expect-error threadId is required.
export const rejectMissingThread = { turnId: "turn", itemId: "item", delta: "delta" } satisfies AgentMessageDeltaNotification;
// @ts-expect-error turnId is required.
export const rejectMissingTurn = { threadId: "thread", itemId: "item", delta: "delta" } satisfies PlanDeltaNotification;
// @ts-expect-error itemId is required.
export const rejectMissingItem = { threadId: "thread", turnId: "turn", delta: "delta" } satisfies AgentMessageDeltaNotification;
// @ts-expect-error delta is required.
export const rejectMissingDelta = { threadId: "thread", turnId: "turn", itemId: "item" } satisfies PlanDeltaNotification;
// @ts-expect-error ids are non-null strings.
export const rejectNullId = { threadId: null, turnId: "turn", itemId: "item", delta: "delta" } satisfies AgentMessageDeltaNotification;
// @ts-expect-error public records exclude live index/time extensions.
export const rejectLiveExtension = { threadId: "thread", turnId: "turn", itemId: "item", delta: "delta", index: 0 } satisfies AgentMessageDeltaNotification;
// @ts-expect-error summaryIndex is required.
export const rejectMissingSummaryIndex = { threadId: "thread", turnId: "turn", itemId: "item" } satisfies ReasoningSummaryPartAddedNotification;
// @ts-expect-error summaryIndex is numeric.
export const rejectStringSummaryIndex = { threadId: "thread", turnId: "turn", itemId: "item", summaryIndex: "0" } satisfies ReasoningSummaryPartAddedNotification;
// @ts-expect-error summary text requires delta.
export const rejectSummaryDelta = { threadId: "thread", turnId: "turn", itemId: "item", summaryIndex: 0 } satisfies ReasoningSummaryTextDeltaNotification;
// @ts-expect-error reasoning text requires contentIndex.
export const rejectMissingContentIndex = { threadId: "thread", turnId: "turn", itemId: "item", delta: "reason" } satisfies ReasoningTextDeltaNotification;
// @ts-expect-error contentIndex is numeric.
export const rejectStringContentIndex = { threadId: "thread", turnId: "turn", itemId: "item", delta: "reason", contentIndex: "0" } satisfies ReasoningTextDeltaNotification;
// @ts-expect-error reasoning text excludes summary indexes.
export const rejectCrossedIndex = { threadId: "thread", turnId: "turn", itemId: "item", delta: "reason", contentIndex: 0, summaryIndex: 0 } satisfies ReasoningTextDeltaNotification;

void (null as unknown as Contracts);
