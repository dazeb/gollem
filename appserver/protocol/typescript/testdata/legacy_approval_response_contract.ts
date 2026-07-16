import type {
  ApplyPatchApprovalResponse,
  CommandExecutionRequestApprovalResponse,
  ExecCommandApprovalResponse,
  FileChangeRequestApprovalResponse,
  MethodParamsByName,
  MethodResultsByName,
  ReviewDecision,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;
type ExpectFalse<T extends false> = T;

type ExactResponse = { decision: ReviewDecision };

export type LegacyApprovalResponseContract = [
  Expect<Equal<ApplyPatchApprovalResponse, ExactResponse>>,
  Expect<Equal<ExecCommandApprovalResponse, ExactResponse>>,
  Expect<Equal<ApplyPatchApprovalResponse, ExecCommandApprovalResponse>>,
  ExpectFalse<Equal<ApplyPatchApprovalResponse, FileChangeRequestApprovalResponse>>,
  ExpectFalse<Equal<ExecCommandApprovalResponse, CommandExecutionRequestApprovalResponse>>,
  ExpectFalse<"applyPatchApproval" extends keyof MethodParamsByName ? true : false>,
  ExpectFalse<"applyPatchApproval" extends keyof MethodResultsByName ? true : false>,
  ExpectFalse<"execCommandApproval" extends keyof MethodParamsByName ? true : false>,
  ExpectFalse<"execCommandApproval" extends keyof MethodResultsByName ? true : false>,
  Expect<Equal<Extract<MethodParamsByName[keyof MethodParamsByName], ApplyPatchApprovalResponse>, never>>,
  Expect<Equal<Extract<MethodResultsByName[keyof MethodResultsByName], ExecCommandApprovalResponse>, never>>,
];

export const approved = { decision: "approved" } satisfies ExecCommandApprovalResponse;
export const session = { decision: "approved_for_session" } satisfies ApplyPatchApprovalResponse;
export const denied = { decision: "denied" } satisfies ExecCommandApprovalResponse;
export const timedOut = { decision: "timed_out" } satisfies ApplyPatchApprovalResponse;
export const abort = { decision: "abort" } satisfies ExecCommandApprovalResponse;
export const execAmendment = {
  decision: {
    approved_execpolicy_amendment: {
      proposed_execpolicy_amendment: ["git", "status"],
    },
  },
} satisfies ApplyPatchApprovalResponse;
export const networkAmendment = {
  decision: {
    network_policy_amendment: {
      network_policy_amendment: { host: "example.invalid", action: "allow" },
    },
  },
} satisfies ExecCommandApprovalResponse;

// @ts-expect-error decision is required.
export const rejectMissing = {} satisfies ExecCommandApprovalResponse;
// @ts-expect-error decision is non-null.
export const rejectNull = { decision: null } satisfies ApplyPatchApprovalResponse;
// @ts-expect-error live decision literals remain distinct.
export const rejectLive = { decision: "accept" } satisfies ExecCommandApprovalResponse;
// @ts-expect-error nested amendments retain their exact shape.
export const rejectAmendment = { decision: { approved_execpolicy_amendment: {} } } satisfies ApplyPatchApprovalResponse;
// @ts-expect-error canonical response records are closed.
export const rejectExtra = { decision: "approved", future: true } satisfies ExecCommandApprovalResponse;
