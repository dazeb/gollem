import type {
  AnalyticsConfig,
  ApprovalsReviewer,
  AskForApproval,
  AutoCompactTokenLimitScope,
  Config,
  ForcedChatgptWorkspaceIds,
  ForcedLoginMethod,
  JsonValue,
  ReasoningEffort,
  ReasoningSummary,
  SandboxMode,
  SandboxWorkspaceWrite,
  ToolsV2,
  Verbosity,
  WebSearchMode,
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

type PublicConfigShape = {
  model: string | null;
  review_model: string | null;
  model_context_window: number | null;
  model_auto_compact_token_limit: number | null;
  model_auto_compact_token_limit_scope: AutoCompactTokenLimitScope | null;
  model_provider: string | null;
  approval_policy: AskForApproval | null;
  approvals_reviewer: ApprovalsReviewer | null;
  sandbox_mode: SandboxMode | null;
  sandbox_workspace_write: SandboxWorkspaceWrite | null;
  forced_chatgpt_workspace_id: ForcedChatgptWorkspaceIds | null;
  forced_login_method: ForcedLoginMethod | null;
  web_search: WebSearchMode | null;
  tools: ToolsV2 | null;
  instructions: string | null;
  developer_instructions: string | null;
  compact_prompt: string | null;
  model_reasoning_effort: ReasoningEffort | null;
  model_reasoning_summary: ReasoningSummary | null;
  model_verbosity: Verbosity | null;
  service_tier: string | null;
  analytics: AnalyticsConfig | null;
  desktop: { [key in string]?: JsonValue } | null;
} & { [key in string]?: JsonValue };

type PublicConfigContract = Expect<Equal<Config, PublicConfigShape>>;

const emptyConfig = {
  model: null,
  review_model: null,
  model_context_window: null,
  model_auto_compact_token_limit: null,
  model_auto_compact_token_limit_scope: null,
  model_provider: null,
  approval_policy: null,
  approvals_reviewer: null,
  sandbox_mode: null,
  sandbox_workspace_write: null,
  forced_chatgpt_workspace_id: null,
  forced_login_method: null,
  web_search: null,
  tools: null,
  instructions: null,
  developer_instructions: null,
  compact_prompt: null,
  model_reasoning_effort: null,
  model_reasoning_summary: null,
  model_verbosity: null,
  service_tier: null,
  analytics: null,
  desktop: null,
} satisfies Config;

({
  ...emptyConfig,
  model: "gpt-5",
  model_context_window: 200000,
  model_auto_compact_token_limit: 180000,
  model_auto_compact_token_limit_scope: "body_after_prefix",
  approval_policy: "on-request",
  approvals_reviewer: "guardian_subagent",
  sandbox_mode: "workspace-write",
  sandbox_workspace_write: {
    writable_roots: ["", "relative", "/absolute"],
    network_access: true,
    exclude_tmpdir_env_var: false,
    exclude_slash_tmp: true,
  },
  forced_chatgpt_workspace_id: ["", "workspace", "workspace"],
  forced_login_method: "api",
  web_search: "live",
  tools: { web_search: null },
  model_reasoning_effort: "xhigh",
  model_reasoning_summary: "detailed",
  model_verbosity: "high",
  analytics: { enabled: true, nested: { value: [9007199254740993, null] } },
  desktop: { theme: "dark", nested: [true, null] },
  apps: { untyped: true },
  future: [9007199254740993, { nested: true }],
}) satisfies Config;

// @ts-expect-error every known field is required nullable.
({}) satisfies Config;
// @ts-expect-error one missing known field is still invalid.
({ model: null }) satisfies Config;
// @ts-expect-error known fields cannot be explicit undefined.
({ ...emptyConfig, model: undefined }) satisfies Config;
// @ts-expect-error context windows are JSON numbers in the generated binding.
({ ...emptyConfig, model_context_window: 1n }) satisfies Config;
// @ts-expect-error context windows exclude strings.
({ ...emptyConfig, model_context_window: "200000" }) satisfies Config;
// @ts-expect-error auto-compact scope is closed.
({ ...emptyConfig, model_auto_compact_token_limit_scope: "bodyAfterPrefix" }) satisfies Config;
// @ts-expect-error approval policy uses the exact public union.
({ ...emptyConfig, approval_policy: "always" }) satisfies Config;
// @ts-expect-error reviewer is closed.
({ ...emptyConfig, approvals_reviewer: "guardian" }) satisfies Config;
// @ts-expect-error sandbox mode is closed.
({ ...emptyConfig, sandbox_mode: "workspace_write" }) satisfies Config;
// @ts-expect-error sandbox config has an exact generated shape.
({ ...emptyConfig, sandbox_workspace_write: { writable_roots: [], network_access: false, exclude_tmpdir_env_var: false, exclude_slash_tmp: false, future: true } }) satisfies Config;
// @ts-expect-error workspace-id arrays contain strings only.
({ ...emptyConfig, forced_chatgpt_workspace_id: ["workspace", 1] }) satisfies Config;
// @ts-expect-error forced login method is closed.
({ ...emptyConfig, forced_login_method: "oauth" }) satisfies Config;
// @ts-expect-error web-search mode is closed.
({ ...emptyConfig, web_search: "enabled" }) satisfies Config;
// @ts-expect-error tools require the exact generated shape.
({ ...emptyConfig, tools: { web_search: false } }) satisfies Config;
// @ts-expect-error instructions are nullable strings.
({ ...emptyConfig, instructions: {} }) satisfies Config;
// @ts-expect-error reasoning summary is closed.
({ ...emptyConfig, model_reasoning_summary: "brief" }) satisfies Config;
// @ts-expect-error verbosity is closed.
({ ...emptyConfig, model_verbosity: "max" }) satisfies Config;
// @ts-expect-error analytics enabled is a nullable boolean.
({ ...emptyConfig, analytics: { enabled: "true" } }) satisfies Config;
// @ts-expect-error desktop values must be JSON.
({ ...emptyConfig, desktop: { callback: () => true } }) satisfies Config;
// @ts-expect-error flattened additional values must be JSON.
({ ...emptyConfig, future: () => true }) satisfies Config;
// @ts-expect-error Config is an object.
null satisfies Config;

declare const publicConfigContract: PublicConfigContract;
void publicConfigContract;
