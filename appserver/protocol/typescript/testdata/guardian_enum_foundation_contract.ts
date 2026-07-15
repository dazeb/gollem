import type {
  GuardianApprovalReviewStatus,
  GuardianCommandSource,
  GuardianRiskLevel,
  GuardianUserAuthorization,
  NetworkApprovalProtocol,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? (<T>() => T extends B ? 1 : 2) extends
        (<T>() => T extends A ? 1 : 2)
      ? true
      : false
    : false;
type Expect<T extends true> = T;

type Contracts = [
  Expect<Equal<GuardianApprovalReviewStatus,
    "inProgress" | "approved" | "denied" | "timedOut" | "aborted">>,
  Expect<Equal<GuardianCommandSource, "shell" | "unifiedExec">>,
  Expect<Equal<GuardianRiskLevel, "low" | "medium" | "high" | "critical">>,
  Expect<Equal<GuardianUserAuthorization, "unknown" | "low" | "medium" | "high">>,
  Expect<Equal<NetworkApprovalProtocol, "http" | "https" | "socks5Tcp" | "socks5Udp">>,
];

"inProgress" satisfies GuardianApprovalReviewStatus;
"approved" satisfies GuardianApprovalReviewStatus;
"denied" satisfies GuardianApprovalReviewStatus;
"timedOut" satisfies GuardianApprovalReviewStatus;
"aborted" satisfies GuardianApprovalReviewStatus;
"shell" satisfies GuardianCommandSource;
"unifiedExec" satisfies GuardianCommandSource;
"low" satisfies GuardianRiskLevel;
"medium" satisfies GuardianRiskLevel;
"high" satisfies GuardianRiskLevel;
"critical" satisfies GuardianRiskLevel;
"unknown" satisfies GuardianUserAuthorization;
"low" satisfies GuardianUserAuthorization;
"medium" satisfies GuardianUserAuthorization;
"high" satisfies GuardianUserAuthorization;
"http" satisfies NetworkApprovalProtocol;
"https" satisfies NetworkApprovalProtocol;
"socks5Tcp" satisfies NetworkApprovalProtocol;
"socks5Udp" satisfies NetworkApprovalProtocol;

// @ts-expect-error review statuses are closed.
"other" satisfies GuardianApprovalReviewStatus;
// @ts-expect-error exact camel-case spelling is required.
"in_progress" satisfies GuardianApprovalReviewStatus;
// @ts-expect-error command sources are closed.
"other" satisfies GuardianCommandSource;
// @ts-expect-error exact camel-case spelling is required.
"unified_exec" satisfies GuardianCommandSource;
// @ts-expect-error risk levels are lowercase.
"Low" satisfies GuardianRiskLevel;
// @ts-expect-error authorization levels are closed.
"other" satisfies GuardianUserAuthorization;
// @ts-expect-error protocols are closed.
"other" satisfies NetworkApprovalProtocol;
// @ts-expect-error exact camel-case spelling is required.
"socks5_tcp" satisfies NetworkApprovalProtocol;
// @ts-expect-error enum values are non-null.
null satisfies GuardianApprovalReviewStatus;
// @ts-expect-error enum values are strings.
1 satisfies NetworkApprovalProtocol;

void (null as unknown as Contracts);
