import type { WebSearchAction } from "../gollem_appserver_protocol";

export const actions = [
  { type: "search", query: null, queries: null },
  { type: "search", query: "gollem", queries: ["gollem", "slang"] },
  { type: "openPage", url: null },
  { type: "openPage", url: "https://example.com" },
  { type: "findInPage", url: null, pattern: null },
  { type: "findInPage", url: "https://example.com", pattern: "needle" },
  { type: "other" },
] satisfies WebSearchAction[];

// @ts-expect-error search query is required nullable, not optional.
export const rejectSearchWithoutQuery = { type: "search", queries: null } satisfies WebSearchAction;
// @ts-expect-error search queries are required nullable, not optional.
export const rejectSearchWithoutQueries = { type: "search", query: null } satisfies WebSearchAction;
// @ts-expect-error query arrays contain only strings.
export const rejectNullQueryElement = { type: "search", query: null, queries: [null] } satisfies WebSearchAction;
// @ts-expect-error openPage URL is required nullable, not optional.
export const rejectOpenPageWithoutURL = { type: "openPage" } satisfies WebSearchAction;
// @ts-expect-error findInPage pattern is required nullable, not optional.
export const rejectFindWithoutPattern = { type: "findInPage", url: null } satisfies WebSearchAction;
// @ts-expect-error variants cannot cross fields.
export const rejectCrossedAction = { type: "other", url: null } satisfies WebSearchAction;
// @ts-expect-error discriminators are closed and camelCase.
export const rejectSnakeCaseType = { type: "open_page", url: null } satisfies WebSearchAction;
