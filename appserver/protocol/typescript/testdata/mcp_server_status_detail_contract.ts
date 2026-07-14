import type { McpServerStatusDetail } from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? (<T>() => T extends B ? 1 : 2) extends
        (<T>() => T extends A ? 1 : 2)
      ? true
      : false
    : false;
type Expect<T extends true> = T;

type Contract = Expect<
  Equal<McpServerStatusDetail, "full" | "toolsAndAuthOnly">
>;

export const details = [
  "full",
  "toolsAndAuthOnly",
] satisfies McpServerStatusDetail[];

// @ts-expect-error status detail values are closed.
export const rejectUnknown = "other" satisfies McpServerStatusDetail;
// @ts-expect-error exact lower-camel spelling is required.
export const rejectFullCase = "Full" satisfies McpServerStatusDetail;
// @ts-expect-error exact lower-camel spelling is required.
export const rejectToolsCase = "ToolsAndAuthOnly" satisfies McpServerStatusDetail;
// @ts-expect-error snake-case detail values are not accepted.
export const rejectSnakeCase = "tools_and_auth_only" satisfies McpServerStatusDetail;
// @ts-expect-error empty strings are not status detail values.
export const rejectEmpty = "" satisfies McpServerStatusDetail;
// @ts-expect-error status detail values are non-null.
export const rejectNull = null satisfies McpServerStatusDetail;
// @ts-expect-error status detail values are strings.
export const rejectNumber = 1 satisfies McpServerStatusDetail;

void (null as unknown as Contract);
