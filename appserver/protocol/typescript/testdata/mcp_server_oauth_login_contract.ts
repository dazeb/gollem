import type {
  McpServerOauthLoginCompletedNotification,
  McpServerOauthLoginParams,
  McpServerOauthLoginResponse,
  MethodParamsByName,
  MethodResultsByName,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2) ? true : false;
type Expect<T extends true> = T;

type Contracts = [
  Expect<Equal<McpServerOauthLoginParams, {
    name: string;
    threadId?: string | null;
    scopes?: Array<string> | null;
    timeoutSecs?: bigint | null;
  }>>,
  Expect<Equal<McpServerOauthLoginResponse, {
    authorizationUrl: string;
  }>>,
  Expect<Equal<McpServerOauthLoginCompletedNotification, {
    name: string;
    threadId: string | null;
    success: boolean;
    error?: string;
  }>>,
  Expect<Equal<"mcpServer/oauth/login" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"mcpServer/oauth/login" extends keyof MethodResultsByName ? true : false, false>>,
  Expect<Equal<"mcpServer/oauthLogin/completed" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"mcpServer/oauthLogin/completed" extends keyof MethodResultsByName ? true : false, false>>,
];

({ name: "" }) satisfies McpServerOauthLoginParams;
({ name: "server", threadId: null, scopes: ["", "scope", "scope"], timeoutSecs: 9223372036854775807n }) satisfies McpServerOauthLoginParams;
({ authorizationUrl: "" }) satisfies McpServerOauthLoginResponse;
({ name: "server", threadId: null, success: true }) satisfies McpServerOauthLoginCompletedNotification;
({ name: "server", threadId: "thread", success: false, error: "" }) satisfies McpServerOauthLoginCompletedNotification;

// @ts-expect-error name is required.
({}) satisfies McpServerOauthLoginParams;
// @ts-expect-error timeout is a signed-int64 bigint in pinned TypeScript.
({ name: "server", timeoutSecs: 1 }) satisfies McpServerOauthLoginParams;
// @ts-expect-error scope elements are strings.
({ name: "server", scopes: [null] }) satisfies McpServerOauthLoginParams;
// @ts-expect-error authorizationUrl is required.
({}) satisfies McpServerOauthLoginResponse;
// @ts-expect-error completion threadId is explicit nullable.
({ name: "server", success: true }) satisfies McpServerOauthLoginCompletedNotification;
// @ts-expect-error completion error omits rather than retaining null.
({ name: "server", threadId: null, success: false, error: null }) satisfies McpServerOauthLoginCompletedNotification;

void (null as unknown as Contracts);
