import type {
  AppConfig,
  AppToolApproval,
  AppToolsConfig,
  ApprovalsReviewer,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2) ? true : false;
type Expect<T extends true> = T;

type Contract = Expect<Equal<AppConfig, {
  approvals_reviewer: ApprovalsReviewer | null;
  default_tools_approval_mode: AppToolApproval | null;
  default_tools_enabled: boolean | null;
  destructive_enabled: boolean | null;
  enabled: boolean;
  open_world_enabled: boolean | null;
  tools: AppToolsConfig | null;
}>>;

({
  approvals_reviewer: null,
  default_tools_approval_mode: null,
  default_tools_enabled: null,
  destructive_enabled: null,
  enabled: true,
  open_world_enabled: null,
  tools: null,
}) satisfies AppConfig;

({
  approvals_reviewer: "guardian_subagent",
  default_tools_approval_mode: "writes",
  default_tools_enabled: true,
  destructive_enabled: true,
  enabled: false,
  open_world_enabled: false,
  tools: {
    "": { approval_mode: null, enabled: null },
    "repos/list": { approval_mode: "prompt", enabled: false },
  },
}) satisfies AppConfig;

// @ts-expect-error canonical AppConfig requires every field.
({ enabled: true }) satisfies AppConfig;
// @ts-expect-error enabled is required and non-null.
({ approvals_reviewer: null, default_tools_approval_mode: null, default_tools_enabled: null, destructive_enabled: null, open_world_enabled: null, tools: null }) satisfies AppConfig;
// @ts-expect-error enabled is non-null.
({ approvals_reviewer: null, default_tools_approval_mode: null, default_tools_enabled: null, destructive_enabled: null, enabled: null, open_world_enabled: null, tools: null }) satisfies AppConfig;
// @ts-expect-error reviewers are closed.
({ approvals_reviewer: "other", default_tools_approval_mode: null, default_tools_enabled: null, destructive_enabled: null, enabled: true, open_world_enabled: null, tools: null }) satisfies AppConfig;
// @ts-expect-error approval modes are closed.
({ approvals_reviewer: null, default_tools_approval_mode: "other", default_tools_enabled: null, destructive_enabled: null, enabled: true, open_world_enabled: null, tools: null }) satisfies AppConfig;
// @ts-expect-error default_tools_enabled is boolean or null.
({ approvals_reviewer: null, default_tools_approval_mode: null, default_tools_enabled: 1, destructive_enabled: null, enabled: true, open_world_enabled: null, tools: null }) satisfies AppConfig;
// @ts-expect-error nested tool values are exact.
({ approvals_reviewer: null, default_tools_approval_mode: null, default_tools_enabled: null, destructive_enabled: null, enabled: true, open_world_enabled: null, tools: { tool: { enabled: true } } }) satisfies AppConfig;
// @ts-expect-error canonical AppConfig is closed.
({ approvals_reviewer: null, default_tools_approval_mode: null, default_tools_enabled: null, destructive_enabled: null, enabled: true, open_world_enabled: null, tools: null, future: true }) satisfies AppConfig;

void (true satisfies Contract);
