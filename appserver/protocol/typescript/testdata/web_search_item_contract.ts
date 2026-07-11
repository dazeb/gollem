import type {
  WebSearchAction,
  WebSearchItem,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type WebSearchItemIsExact = Expect<
  Equal<
    WebSearchItem,
    { id: string; query: string; action: WebSearchAction | null }
  >
>;

export const items = [
  { id: "", query: "", action: null },
  {
    id: "search-1",
    query: "gollem",
    action: { type: "search", query: "gollem", queries: ["gollem", "slang"] },
  },
  { id: "search-2", query: "https://example.com", action: { type: "openPage", url: null } },
  {
    id: "search-3",
    query: "needle",
    action: { type: "findInPage", url: "https://example.com", pattern: "needle" },
  },
  { id: "search-4", query: "custom", action: { type: "other" } },
] satisfies WebSearchItem[];

// @ts-expect-error id is required.
export const rejectMissingId = { query: "q", action: null } satisfies WebSearchItem;
// @ts-expect-error query is required.
export const rejectMissingQuery = { id: "id", action: null } satisfies WebSearchItem;
// @ts-expect-error action is required nullable, not optional.
export const rejectMissingAction = { id: "id", query: "q" } satisfies WebSearchItem;
// @ts-expect-error id is non-null.
export const rejectNullId = { id: null, query: "q", action: null } satisfies WebSearchItem;
// @ts-expect-error query is non-null.
export const rejectNullQuery = { id: "id", query: null, action: null } satisfies WebSearchItem;
// @ts-expect-error nested actions preserve their required nullable fields.
export const rejectMalformedAction = { id: "id", query: "q", action: { type: "search", query: null } } satisfies WebSearchItem;
// @ts-expect-error WebSearchItem uses the camel-case v2 action, not the snake-case Responses API action.
export const rejectSnakeCaseAction = { id: "id", query: "q", action: { type: "open_page", url: "https://example.com" } } satisfies WebSearchItem;
// @ts-expect-error the item object is closed.
export const rejectUnknownField = { id: "id", query: "q", action: null, extra: true } satisfies WebSearchItem;

declare const webSearchItemIsExact: WebSearchItemIsExact;
void webSearchItemIsExact;
