import type {
  AnalyticsConfig,
  AutoCompactTokenLimitScope,
  ForcedChatgptWorkspaceIds,
  ForcedLoginMethod,
  JsonValue,
  SandboxWorkspaceWrite,
  ToolsV2,
  Verbosity,
  WebSearchContextSize,
  WebSearchLocation,
  WebSearchToolConfig,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? (<T>() => T extends B ? 1 : 2) extends
        (<T>() => T extends A ? 1 : 2)
      ? true
      : false
    : false;
type Expect<T extends true> = T;

type ConfigPrerequisiteContracts = [
  Expect<Equal<AutoCompactTokenLimitScope, "total" | "body_after_prefix">>,
  Expect<Equal<ForcedLoginMethod, "chatgpt" | "api">>,
  Expect<Equal<Verbosity, "low" | "medium" | "high">>,
  Expect<Equal<WebSearchContextSize, "low" | "medium" | "high">>,
  Expect<Equal<ForcedChatgptWorkspaceIds, string | Array<string>>>,
  Expect<Equal<AnalyticsConfig, {
    enabled: boolean | null;
  } & { [key in string]?: JsonValue }>>,
  Expect<Equal<SandboxWorkspaceWrite, {
    writable_roots: Array<string>;
    network_access: boolean;
    exclude_tmpdir_env_var: boolean;
    exclude_slash_tmp: boolean;
  }>>,
  Expect<Equal<WebSearchLocation, {
    country: string | null;
    region: string | null;
    city: string | null;
    timezone: string | null;
  }>>,
  Expect<Equal<WebSearchToolConfig, {
    context_size: WebSearchContextSize | null;
    allowed_domains: Array<string> | null;
    location: WebSearchLocation | null;
  }>>,
  Expect<Equal<ToolsV2, { web_search: WebSearchToolConfig | null }>>,
];

"total" satisfies AutoCompactTokenLimitScope;
"body_after_prefix" satisfies AutoCompactTokenLimitScope;
"chatgpt" satisfies ForcedLoginMethod;
"api" satisfies ForcedLoginMethod;
"low" satisfies Verbosity;
"medium" satisfies Verbosity;
"high" satisfies Verbosity;
"low" satisfies WebSearchContextSize;
"medium" satisfies WebSearchContextSize;
"high" satisfies WebSearchContextSize;
"" satisfies ForcedChatgptWorkspaceIds;
[] satisfies ForcedChatgptWorkspaceIds;
["", "workspace", "workspace"] satisfies ForcedChatgptWorkspaceIds;
({ enabled: null }) satisfies AnalyticsConfig;
({ enabled: false, nested: { value: [9007199254740993, true, null] } }) satisfies AnalyticsConfig;
({
  writable_roots: ["", "relative", "/absolute"],
  network_access: false,
  exclude_tmpdir_env_var: false,
  exclude_slash_tmp: true,
}) satisfies SandboxWorkspaceWrite;
({ country: null, region: "", city: "New York", timezone: "America/New_York" }) satisfies WebSearchLocation;
({ context_size: null, allowed_domains: null, location: null }) satisfies WebSearchToolConfig;
({
  context_size: "high",
  allowed_domains: ["", "example.com", "example.com"],
  location: { country: "US", region: null, city: null, timezone: null },
}) satisfies WebSearchToolConfig;
({ web_search: null }) satisfies ToolsV2;
({ web_search: { context_size: null, allowed_domains: [], location: null } }) satisfies ToolsV2;

// @ts-expect-error auto-compact scope is closed.
"bodyAfterPrefix" satisfies AutoCompactTokenLimitScope;
// @ts-expect-error forced login method is closed.
"oauth" satisfies ForcedLoginMethod;
// @ts-expect-error verbosity is closed.
"max" satisfies Verbosity;
// @ts-expect-error web-search context size is closed.
"large" satisfies WebSearchContextSize;

// @ts-expect-error workspace ids exclude null.
null satisfies ForcedChatgptWorkspaceIds;
// @ts-expect-error workspace-id arrays contain strings only.
["workspace", 1] satisfies ForcedChatgptWorkspaceIds;
// @ts-expect-error workspace ids exclude object forms.
({ workspace: "id" }) satisfies ForcedChatgptWorkspaceIds;

// @ts-expect-error enabled is required nullable.
({}) satisfies AnalyticsConfig;
// @ts-expect-error enabled is a nullable boolean.
({ enabled: "true" }) satisfies AnalyticsConfig;
// @ts-expect-error enabled cannot be explicit undefined.
({ enabled: undefined }) satisfies AnalyticsConfig;
// @ts-expect-error additional analytics values must be JSON.
({ enabled: null, callback: () => true }) satisfies AnalyticsConfig;

// @ts-expect-error sandbox fields are required despite decoder defaults.
({}) satisfies SandboxWorkspaceWrite;
// @ts-expect-error writable roots are non-null.
({ writable_roots: null, network_access: false, exclude_tmpdir_env_var: false, exclude_slash_tmp: false }) satisfies SandboxWorkspaceWrite;
// @ts-expect-error writable-root members are strings.
({ writable_roots: [1], network_access: false, exclude_tmpdir_env_var: false, exclude_slash_tmp: false }) satisfies SandboxWorkspaceWrite;
// @ts-expect-error network access is boolean.
({ writable_roots: [], network_access: null, exclude_tmpdir_env_var: false, exclude_slash_tmp: false }) satisfies SandboxWorkspaceWrite;
// @ts-expect-error generated sandbox TypeScript remains closed.
({ writable_roots: [], network_access: false, exclude_tmpdir_env_var: false, exclude_slash_tmp: false, future: true }) satisfies SandboxWorkspaceWrite;

// @ts-expect-error location fields are required nullable.
({}) satisfies WebSearchLocation;
// @ts-expect-error location fields are nullable strings.
({ country: 1, region: null, city: null, timezone: null }) satisfies WebSearchLocation;
// @ts-expect-error location is closed.
({ country: null, region: null, city: null, timezone: null, extra: true }) satisfies WebSearchLocation;

// @ts-expect-error tool fields are required nullable.
({}) satisfies WebSearchToolConfig;
// @ts-expect-error domain members are strings.
({ context_size: null, allowed_domains: [null], location: null }) satisfies WebSearchToolConfig;
// @ts-expect-error context size uses the closed enum.
({ context_size: "large", allowed_domains: null, location: null }) satisfies WebSearchToolConfig;
// @ts-expect-error web-search tool config is closed.
({ context_size: null, allowed_domains: null, location: null, extra: true }) satisfies WebSearchToolConfig;

// @ts-expect-error web_search is required nullable.
({}) satisfies ToolsV2;
// @ts-expect-error web_search is nullable config, not boolean.
({ web_search: false }) satisfies ToolsV2;
// @ts-expect-error generated tools TypeScript remains closed.
({ web_search: null, future: true }) satisfies ToolsV2;

declare const configPrerequisiteContracts: ConfigPrerequisiteContracts;
void configPrerequisiteContracts;
