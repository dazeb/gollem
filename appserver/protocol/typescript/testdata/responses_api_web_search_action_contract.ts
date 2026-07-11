import type { ResponsesApiWebSearchAction } from "../gollem_appserver_protocol";

export const actions = [
  { type: "search" },
  { type: "search", query: "gollem" },
  { type: "search", queries: [] },
  { type: "search", query: "gollem", queries: ["gollem", "slang"] },
  { type: "open_page" },
  { type: "open_page", url: "https://example.com" },
  { type: "find_in_page" },
  { type: "find_in_page", url: "https://example.com", pattern: "needle" },
  { type: "other" },
] satisfies ResponsesApiWebSearchAction[];

// @ts-expect-error optional query is non-null when present.
export const rejectNullQuery = { type: "search", query: null } satisfies ResponsesApiWebSearchAction;
// @ts-expect-error optional queries are non-null when present.
export const rejectNullQueries = { type: "search", queries: null } satisfies ResponsesApiWebSearchAction;
// @ts-expect-error query arrays contain only strings.
export const rejectNullQueryElement = { type: "search", queries: [null] } satisfies ResponsesApiWebSearchAction;
// @ts-expect-error variants cannot cross fields.
export const rejectCrossedAction = { type: "other", url: "https://example.com" } satisfies ResponsesApiWebSearchAction;
// @ts-expect-error Responses API discriminators are snake_case.
export const rejectCamelCaseType = { type: "openPage", url: "https://example.com" } satisfies ResponsesApiWebSearchAction;
