import type { McpAuthStatus } from "../gollem_appserver_protocol";

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
  Equal<
    McpAuthStatus,
    "unsupported" | "notLoggedIn" | "bearerToken" | "oAuth"
  >
>;

export const statuses = [
  "unsupported",
  "notLoggedIn",
  "bearerToken",
  "oAuth",
] satisfies McpAuthStatus[];

// @ts-expect-error MCP auth statuses are closed.
export const rejectUnknown = "other" satisfies McpAuthStatus;
// @ts-expect-error exact lower-camel unsupported spelling is required.
export const rejectUnsupportedCase = "Unsupported" satisfies McpAuthStatus;
// @ts-expect-error exact lower-camel not-logged-in spelling is required.
export const rejectSnakeCase = "not_logged_in" satisfies McpAuthStatus;
// @ts-expect-error exact mixed-case OAuth spelling is required.
export const rejectOAuthCase = "oauth" satisfies McpAuthStatus;
// @ts-expect-error empty strings are not MCP auth statuses.
export const rejectEmpty = "" satisfies McpAuthStatus;
// @ts-expect-error MCP auth statuses are non-null.
export const rejectNull = null satisfies McpAuthStatus;
// @ts-expect-error MCP auth statuses are strings.
export const rejectNumber = 1 satisfies McpAuthStatus;

void (null as unknown as Contract);
