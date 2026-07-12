import type {
  SortDirection,
  ThreadResumeInitialTurnsPageParams,
  Turn,
  TurnItemsView,
  TurnsPage,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type ParamsExpected = {
  limit?: number | null;
  sortDirection?: SortDirection | null;
  itemsView?: TurnItemsView | null;
};
type PageExpected = {
  data: Turn[];
  nextCursor: string | null;
  backwardsCursor: string | null;
};
type Contracts = [
  Expect<Equal<ThreadResumeInitialTurnsPageParams, ParamsExpected>>,
  Expect<Equal<TurnsPage, PageExpected>>,
];

export const empty = {} satisfies ThreadResumeInitialTurnsPageParams;
export const nullable = {
  limit: null,
  sortDirection: null,
  itemsView: null,
} satisfies ThreadResumeInitialTurnsPageParams;
export const populated = {
  limit: 100,
  sortDirection: "desc",
  itemsView: "summary",
} satisfies ThreadResumeInitialTurnsPageParams;

declare const turn: Turn;
export const emptyPage = {
  data: [],
  nextCursor: null,
  backwardsCursor: null,
} satisfies TurnsPage;
export const populatedPage = {
  data: [turn],
  nextCursor: "next",
  backwardsCursor: "back",
} satisfies TurnsPage;

// @ts-expect-error limit is numeric when present.
export const rejectStringLimit = { limit: "100" } satisfies ThreadResumeInitialTurnsPageParams;
// @ts-expect-error sort direction remains closed.
export const rejectSortDirection = { sortDirection: "ascending" } satisfies ThreadResumeInitialTurnsPageParams;
// @ts-expect-error item detail remains closed.
export const rejectItemsView = { itemsView: "all" } satisfies ThreadResumeInitialTurnsPageParams;
// @ts-expect-error params exclude pagination aliases.
export const rejectPageSize = { pageSize: 100 } satisfies ThreadResumeInitialTurnsPageParams;
// @ts-expect-error data is required.
export const rejectMissingData = { nextCursor: null, backwardsCursor: null } satisfies TurnsPage;
// @ts-expect-error data is non-null.
export const rejectNullData = { data: null, nextCursor: null, backwardsCursor: null } satisfies TurnsPage;
// @ts-expect-error canonical TypeScript requires nextCursor.
export const rejectMissingNext = { data: [], backwardsCursor: null } satisfies TurnsPage;
// @ts-expect-error canonical TypeScript requires backwardsCursor.
export const rejectMissingBackwards = { data: [], nextCursor: null } satisfies TurnsPage;
// @ts-expect-error cursors are nullable strings only.
export const rejectNumericCursor = { data: [], nextCursor: 1, backwardsCursor: null } satisfies TurnsPage;
// @ts-expect-error nested turns remain strict.
export const rejectMalformedTurn = { data: [{}], nextCursor: null, backwardsCursor: null } satisfies TurnsPage;
// @ts-expect-error page objects are closed.
export const rejectUnknown = { data: [], nextCursor: null, backwardsCursor: null, cursor: "next" } satisfies TurnsPage;

void (null as unknown as Contracts);
