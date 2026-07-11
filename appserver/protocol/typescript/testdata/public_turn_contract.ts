import type {
  ThreadItem,
  Turn,
  TurnError,
  TurnItemsView,
  TurnStatus,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type TurnIsExact = Expect<
  Equal<
    Turn,
    {
      id: string;
      items: ThreadItem[];
      itemsView: TurnItemsView;
      status: TurnStatus;
      error: TurnError | null;
      startedAt: number | null;
      completedAt: number | null;
      durationMs: number | null;
    }
  >
>;

export const turns = [
  {
    id: "",
    items: [],
    itemsView: "notLoaded",
    status: "completed",
    error: null,
    startedAt: null,
    completedAt: null,
    durationMs: null,
  },
  {
    id: "turn-1",
    items: [{ type: "contextCompaction", id: "item-1" }],
    itemsView: "full",
    status: "failed",
    error: { message: "failed", codexErrorInfo: "sandboxError", additionalDetails: null },
    startedAt: -1,
    completedAt: 0,
    durationMs: -2,
  },
] satisfies Turn[];

// @ts-expect-error every nullable field remains required
export const missingError: Turn = {
  id: "turn",
  items: [],
  itemsView: "full",
  status: "completed",
  startedAt: null,
  completedAt: null,
  durationMs: null,
};

export const malformedTurns = [
  // @ts-expect-error items cannot be null
  { id: "turn", items: null, itemsView: "full", status: "completed", error: null, startedAt: null, completedAt: null, durationMs: null },
  // @ts-expect-error itemsView is closed
  { id: "turn", items: [], itemsView: "other", status: "completed", error: null, startedAt: null, completedAt: null, durationMs: null },
  // @ts-expect-error status is closed
  { id: "turn", items: [], itemsView: "full", status: "running", error: null, startedAt: null, completedAt: null, durationMs: null },
] satisfies Turn[];
