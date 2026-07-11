import type {
  CommandExecutionApprovalDecision,
  CommandExecutionRequestApprovalResponse,
  ExecPolicyAmendment,
  NetworkPolicyAmendment,
  NetworkPolicyRuleAction,
  ResultFor,
} from "../gollem_appserver_protocol";

export const execPolicy = ["git", "status"] satisfies ExecPolicyAmendment;
export const networkAction = "allow" satisfies NetworkPolicyRuleAction;
export const networkPolicy = {
  host: "example.com",
  action: networkAction,
} satisfies NetworkPolicyAmendment;
export const decisions = [
  "accept",
  "acceptForSession",
  { acceptWithExecpolicyAmendment: { execpolicy_amendment: execPolicy } },
  { applyNetworkPolicyAmendment: { network_policy_amendment: networkPolicy } },
  "decline",
  "cancel",
] satisfies CommandExecutionApprovalDecision[];
export const response = {
  decision: decisions[0]!,
} satisfies CommandExecutionRequestApprovalResponse;
response satisfies ResultFor<"item/commandExecution/requestApproval">;

// @ts-expect-error command decisions are closed.
"approve" satisfies CommandExecutionApprovalDecision;
// @ts-expect-error exec-policy amendments require the public snake-case field.
({ acceptWithExecpolicyAmendment: { execpolicyAmendment: execPolicy } }) satisfies CommandExecutionApprovalDecision;
// @ts-expect-error network policy actions are closed.
({ host: "example.com", action: "prompt" }) satisfies NetworkPolicyAmendment;
// @ts-expect-error responses require a decision.
({}) satisfies CommandExecutionRequestApprovalResponse;
