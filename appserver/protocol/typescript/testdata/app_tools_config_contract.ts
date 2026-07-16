import type {
  AppToolApproval,
  AppToolConfig,
  AppToolsConfig,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2) ? true : false;
type Expect<T extends true> = T;

type Contracts = [
  Expect<Equal<AppToolConfig, {
    approval_mode: AppToolApproval | null;
    enabled: boolean | null;
  }>>,
  Expect<Equal<AppToolsConfig, {
    [key in string]?: {
      approval_mode: AppToolApproval | null;
      enabled: boolean | null;
    };
  }>>,
];

({
  approval_mode: null,
  enabled: null,
}) satisfies AppToolConfig;

({
  "": { approval_mode: "auto", enabled: true },
  " ": { approval_mode: "prompt", enabled: false },
  "repos/list": { approval_mode: "writes", enabled: null },
  opaque: { approval_mode: null, enabled: null },
}) satisfies AppToolsConfig;

({}) satisfies AppToolsConfig;
({ tool: undefined }) satisfies AppToolsConfig;

// @ts-expect-error canonical AppToolConfig requires both nullable fields.
({}) satisfies AppToolConfig;
// @ts-expect-error enabled is required even when approval_mode is null.
({ approval_mode: null }) satisfies AppToolConfig;
// @ts-expect-error approval_mode is required even when enabled is null.
({ enabled: null }) satisfies AppToolConfig;
// @ts-expect-error approval modes are closed.
({ approval_mode: "other", enabled: true }) satisfies AppToolConfig;
// @ts-expect-error enabled is boolean or null.
({ approval_mode: "approve", enabled: 1 }) satisfies AppToolConfig;
// @ts-expect-error canonical AppToolConfig has no extra fields.
({ approval_mode: null, enabled: null, future: true }) satisfies AppToolConfig;
// @ts-expect-error tool entries cannot be null.
({ tool: null }) satisfies AppToolsConfig;
// @ts-expect-error each tool entry requires both canonical fields.
({ tool: { enabled: true } }) satisfies AppToolsConfig;
void (null as unknown as Contracts);
