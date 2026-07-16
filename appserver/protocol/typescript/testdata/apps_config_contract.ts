import type {
  AppConfig,
  AppToolApproval,
  AppToolsConfig,
  ApprovalsReviewer,
  AppsConfig,
  AppsDefaultConfig,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2) ? true : false;
type Expect<T extends true> = T;

type ExpectedDefault = {
  approvals_reviewer: ApprovalsReviewer | null;
  default_tools_approval_mode: AppToolApproval | null;
  destructive_enabled: boolean;
  enabled: boolean;
  open_world_enabled: boolean;
};
type ExpectedApp = {
  approvals_reviewer: ApprovalsReviewer | null;
  default_tools_approval_mode: AppToolApproval | null;
  default_tools_enabled: boolean | null;
  destructive_enabled: boolean | null;
  enabled: boolean;
  open_world_enabled: boolean | null;
  tools: AppToolsConfig | null;
};
type ExpectedApps = { _default: AppsDefaultConfig | null } & {
  [key in string]?: ExpectedApp;
};
type Contracts = [
  Expect<Equal<AppsDefaultConfig, ExpectedDefault>>,
  Expect<Equal<AppConfig, ExpectedApp>>,
  Expect<Equal<AppsConfig, ExpectedApps>>,
];

({
  approvals_reviewer: null,
  default_tools_approval_mode: null,
  destructive_enabled: true,
  enabled: true,
  open_world_enabled: true,
}) satisfies AppsDefaultConfig;

({
  approvals_reviewer: "guardian_subagent",
  default_tools_approval_mode: "writes",
  destructive_enabled: false,
  enabled: false,
  open_world_enabled: false,
}) satisfies AppsDefaultConfig;

declare const apps: AppsConfig;
apps._default satisfies AppsDefaultConfig | null;
apps["arbitrary"] satisfies AppConfig | undefined;

// @ts-expect-error canonical defaults require every field.
({ enabled: true }) satisfies AppsDefaultConfig;
// @ts-expect-error booleans are non-null.
({ approvals_reviewer: null, default_tools_approval_mode: null, destructive_enabled: null, enabled: true, open_world_enabled: true }) satisfies AppsDefaultConfig;
// @ts-expect-error reviewers are closed.
({ approvals_reviewer: "other", default_tools_approval_mode: null, destructive_enabled: true, enabled: true, open_world_enabled: true }) satisfies AppsDefaultConfig;
// @ts-expect-error approval modes are closed.
({ approvals_reviewer: null, default_tools_approval_mode: "other", destructive_enabled: true, enabled: true, open_world_enabled: true }) satisfies AppsDefaultConfig;
// @ts-expect-error pinned flattened intersection requires _default.
({}) satisfies AppsConfig;
// @ts-expect-error pinned mapped values are strict AppConfig records.
({ _default: null, app: { enabled: true } }) satisfies AppsConfig;
// @ts-expect-error flattened app values cannot be null.
({ _default: null, app: null }) satisfies AppsConfig;
// @ts-expect-error flattened app values preserve closed reviewer values.
({ _default: null, app: { approvals_reviewer: "other", default_tools_approval_mode: null, default_tools_enabled: null, destructive_enabled: null, enabled: true, open_world_enabled: null, tools: null } }) satisfies AppsConfig;

void (null as unknown as Contracts);
