import type {
  JsonValue,
  McpServerInfo,
  MethodParamsByName,
  MethodResultsByName,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type Expected = {
  name: string;
  title: string | null;
  version: string;
  description: string | null;
  icons: Array<JsonValue> | null;
  websiteUrl: string | null;
};
type Contracts = [
  Expect<Equal<McpServerInfo, Expected>>,
  Expect<Equal<"mcpServerStatus/list" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"mcpServerStatus/list" extends keyof MethodResultsByName ? true : false, false>>,
];

export const empty = {
  name: "",
  title: null,
  version: "",
  description: null,
  icons: null,
  websiteUrl: null,
} satisfies McpServerInfo;
export const full = {
  name: "server",
  title: "Server",
  version: "1.0.0",
  description: "Description",
  icons: [null, true, "icon", 9007199254740993, { source: ["client", null] }],
  websiteUrl: "https://example.com",
} satisfies McpServerInfo;
export const emptyIcons = {
  name: "server",
  title: "",
  version: "1.0.0",
  description: "",
  icons: [],
  websiteUrl: "",
} satisfies McpServerInfo;

// @ts-expect-error name is required.
export const rejectMissingName = { title: null, version: "1", description: null, icons: null, websiteUrl: null } satisfies McpServerInfo;
// @ts-expect-error name is non-null.
export const rejectNullName = { name: null, title: null, version: "1", description: null, icons: null, websiteUrl: null } satisfies McpServerInfo;
// @ts-expect-error canonical title is required nullable.
export const rejectMissingTitle = { name: "server", version: "1", description: null, icons: null, websiteUrl: null } satisfies McpServerInfo;
// @ts-expect-error title cannot be explicitly undefined.
export const rejectUndefinedTitle = { name: "server", title: undefined, version: "1", description: null, icons: null, websiteUrl: null } satisfies McpServerInfo;
// @ts-expect-error version is required.
export const rejectMissingVersion = { name: "server", title: null, description: null, icons: null, websiteUrl: null } satisfies McpServerInfo;
// @ts-expect-error version is non-null.
export const rejectNullVersion = { name: "server", title: null, version: null, description: null, icons: null, websiteUrl: null } satisfies McpServerInfo;
// @ts-expect-error description is string or null.
export const rejectDescriptionObject = { name: "server", title: null, version: "1", description: {}, icons: null, websiteUrl: null } satisfies McpServerInfo;
// @ts-expect-error icons is an array or null.
export const rejectIconsObject = { name: "server", title: null, version: "1", description: null, icons: {}, websiteUrl: null } satisfies McpServerInfo;
// @ts-expect-error functions are not JSON icon values.
export const rejectIconFunction = { name: "server", title: null, version: "1", description: null, icons: [() => true], websiteUrl: null } satisfies McpServerInfo;
// @ts-expect-error exact camel-case website field is required.
export const rejectWebsiteAlias = { name: "server", title: null, version: "1", description: null, icons: null, website_url: null } satisfies McpServerInfo;
// @ts-expect-error fields absent from the public record are rejected.
export const rejectExtra = { name: "server", title: null, version: "1", description: null, icons: null, websiteUrl: null, extra: true } satisfies McpServerInfo;

void (null as unknown as Contracts);
