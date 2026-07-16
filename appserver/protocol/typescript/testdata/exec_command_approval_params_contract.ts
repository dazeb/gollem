import type {
  CommandExecutionApprovalRequestParams,
  ExecCommandApprovalParams,
  MethodParamsByName,
  MethodResultsByName,
  ParsedCommand,
  ThreadId,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;
type ExpectFalse<T extends false> = T;

type ExactParams = {
  conversationId: ThreadId;
  callId: string;
  approvalId: string | null;
  command: Array<string>;
  cwd: string;
  reason: string | null;
  parsedCmd: Array<ParsedCommand>;
};

export type ExecCommandApprovalParamsContract = [
  Expect<Equal<ExecCommandApprovalParams, ExactParams>>,
  ExpectFalse<Equal<ExecCommandApprovalParams, CommandExecutionApprovalRequestParams>>,
  ExpectFalse<"execCommandApproval" extends keyof MethodParamsByName ? true : false>,
  ExpectFalse<"execCommandApproval" extends keyof MethodResultsByName ? true : false>,
  Expect<Equal<Extract<MethodParamsByName[keyof MethodParamsByName], ExecCommandApprovalParams>, never>>,
  Expect<Equal<Extract<MethodResultsByName[keyof MethodResultsByName], ExecCommandApprovalParams>, never>>,
];

({
  conversationId: "",
  callId: "",
  approvalId: null,
  command: [],
  cwd: "",
  reason: null,
  parsedCmd: [],
}) satisfies ExecCommandApprovalParams;

({
  conversationId: "thread",
  callId: "call",
  approvalId: "approval",
  command: ["rg", "q"],
  cwd: "relative/worktree",
  reason: "inspect",
  parsedCmd: [{ type: "search", cmd: "rg q", query: "q", path: null }],
}) satisfies ExecCommandApprovalParams;

// @ts-expect-error conversationId is required.
({ callId: "call", approvalId: null, command: [], cwd: ".", reason: null, parsedCmd: [] }) satisfies ExecCommandApprovalParams;
// @ts-expect-error callId is required.
({ conversationId: "thread", approvalId: null, command: [], cwd: ".", reason: null, parsedCmd: [] }) satisfies ExecCommandApprovalParams;
// @ts-expect-error approvalId is canonical-required nullable.
({ conversationId: "thread", callId: "call", command: [], cwd: ".", reason: null, parsedCmd: [] }) satisfies ExecCommandApprovalParams;
// @ts-expect-error command is required.
({ conversationId: "thread", callId: "call", approvalId: null, cwd: ".", reason: null, parsedCmd: [] }) satisfies ExecCommandApprovalParams;
// @ts-expect-error command elements are strings.
({ conversationId: "thread", callId: "call", approvalId: null, command: [1], cwd: ".", reason: null, parsedCmd: [] }) satisfies ExecCommandApprovalParams;
// @ts-expect-error cwd is required.
({ conversationId: "thread", callId: "call", approvalId: null, command: [], reason: null, parsedCmd: [] }) satisfies ExecCommandApprovalParams;
// @ts-expect-error reason is canonical-required nullable.
({ conversationId: "thread", callId: "call", approvalId: null, command: [], cwd: ".", parsedCmd: [] }) satisfies ExecCommandApprovalParams;
// @ts-expect-error parsedCmd is required.
({ conversationId: "thread", callId: "call", approvalId: null, command: [], cwd: ".", reason: null }) satisfies ExecCommandApprovalParams;
// @ts-expect-error nested parsed commands remain strict.
({ conversationId: "thread", callId: "call", approvalId: null, command: [], cwd: ".", reason: null, parsedCmd: [{ type: "unknown" }] }) satisfies ExecCommandApprovalParams;
// @ts-expect-error canonical records are closed.
({ conversationId: "thread", callId: "call", approvalId: null, command: [], cwd: ".", reason: null, parsedCmd: [], future: true }) satisfies ExecCommandApprovalParams;
