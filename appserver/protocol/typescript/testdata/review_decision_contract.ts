import type {
  CommandExecutionApprovalDecision,
  ExecPolicyAmendment,
  MethodParamsByName,
  MethodResultsByName,
  NetworkPolicyAmendment,
  ReviewDecision,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;
type ExpectFalse<T extends false> = T;

type ExactDecision =
  | "approved"
  | {
      approved_execpolicy_amendment: {
        proposed_execpolicy_amendment: ExecPolicyAmendment;
      };
    }
  | "approved_for_session"
  | {
      network_policy_amendment: {
        network_policy_amendment: NetworkPolicyAmendment;
      };
    }
  | "denied"
  | "timed_out"
  | "abort";

export type ReviewDecisionContract = [
  Expect<Equal<ReviewDecision, ExactDecision>>,
  ExpectFalse<Equal<ReviewDecision, CommandExecutionApprovalDecision>>,
  ExpectFalse<"execCommandApproval" extends keyof MethodParamsByName ? true : false>,
  ExpectFalse<"execCommandApproval" extends keyof MethodResultsByName ? true : false>,
  ExpectFalse<"applyPatchApproval" extends keyof MethodParamsByName ? true : false>,
  ExpectFalse<"applyPatchApproval" extends keyof MethodResultsByName ? true : false>,
  Expect<Equal<Extract<MethodParamsByName[keyof MethodParamsByName], ReviewDecision>, never>>,
  Expect<Equal<Extract<MethodResultsByName[keyof MethodResultsByName], ReviewDecision>, never>>,
];

export const approved = "approved" satisfies ReviewDecision;
export const approvedForSession = "approved_for_session" satisfies ReviewDecision;
export const denied = "denied" satisfies ReviewDecision;
export const timedOut = "timed_out" satisfies ReviewDecision;
export const abort = "abort" satisfies ReviewDecision;
export const execAmendment = {
  approved_execpolicy_amendment: {
    proposed_execpolicy_amendment: ["git", "status"],
  },
} satisfies ReviewDecision;
export const networkAmendment = {
  network_policy_amendment: {
    network_policy_amendment: { host: "example.invalid", action: "allow" },
  },
} satisfies ReviewDecision;

// @ts-expect-error live literals are not legacy review decisions.
export const rejectLiveLiteral = "accept" satisfies ReviewDecision;
// @ts-expect-error snake-case discriminant is exact.
export const rejectCase = "approvedForSession" satisfies ReviewDecision;
// @ts-expect-error proposed amendment is required.
export const rejectMissingExecAmendment = { approved_execpolicy_amendment: {} } satisfies ReviewDecision;
// @ts-expect-error proposed amendment elements are strings.
export const rejectExecElement = { approved_execpolicy_amendment: { proposed_execpolicy_amendment: [1] } } satisfies ReviewDecision;
// @ts-expect-error network amendment wrapper is required.
export const rejectMissingNetworkAmendment = { network_policy_amendment: {} } satisfies ReviewDecision;
// @ts-expect-error network policy action is closed.
export const rejectNetworkAction = { network_policy_amendment: { network_policy_amendment: { host: "h", action: "prompt" } } } satisfies ReviewDecision;
// @ts-expect-error object variants are closed.
export const rejectOuterExtra = { approved_execpolicy_amendment: { proposed_execpolicy_amendment: [] }, future: true } satisfies ReviewDecision;
// @ts-expect-error nested canonical objects are closed.
export const rejectNestedExtra = { approved_execpolicy_amendment: { proposed_execpolicy_amendment: [], future: true } } satisfies ReviewDecision;
// @ts-expect-error null is not a review decision.
export const rejectNull = null satisfies ReviewDecision;
