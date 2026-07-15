import type {
  GuardianApprovalReview,
  GuardianApprovalReviewStatus,
  GuardianRiskLevel,
  GuardianUserAuthorization,
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
  Expect<Equal<GuardianApprovalReview, {
    status: GuardianApprovalReviewStatus;
    riskLevel: GuardianRiskLevel | null;
    userAuthorization: GuardianUserAuthorization | null;
    rationale: string | null;
  }>>,
];

({
  status: "inProgress",
  riskLevel: null,
  userAuthorization: null,
  rationale: null,
}) satisfies GuardianApprovalReview;
({
  status: "denied",
  riskLevel: "critical",
  userAuthorization: "unknown",
  rationale: "",
}) satisfies GuardianApprovalReview;
({
  status: "timedOut",
  riskLevel: "low",
  userAuthorization: "high",
  rationale: "opaque\nvalue",
}) satisfies GuardianApprovalReview;

// @ts-expect-error status is required.
({ riskLevel: null, userAuthorization: null, rationale: null }) satisfies GuardianApprovalReview;
// @ts-expect-error status is non-null.
({ status: null, riskLevel: null, userAuthorization: null, rationale: null }) satisfies GuardianApprovalReview;
// @ts-expect-error statuses are closed.
({ status: "other", riskLevel: null, userAuthorization: null, rationale: null }) satisfies GuardianApprovalReview;
// @ts-expect-error riskLevel is required by exact ts-rs output.
({ status: "approved", userAuthorization: null, rationale: null }) satisfies GuardianApprovalReview;
// @ts-expect-error risk levels are closed.
({ status: "approved", riskLevel: "other", userAuthorization: null, rationale: null }) satisfies GuardianApprovalReview;
// @ts-expect-error userAuthorization is required by exact ts-rs output.
({ status: "approved", riskLevel: null, rationale: null }) satisfies GuardianApprovalReview;
// @ts-expect-error authorization levels are closed.
({ status: "approved", riskLevel: null, userAuthorization: "other", rationale: null }) satisfies GuardianApprovalReview;
// @ts-expect-error rationale is required by exact ts-rs output.
({ status: "approved", riskLevel: null, userAuthorization: null }) satisfies GuardianApprovalReview;
// @ts-expect-error rationale is nullable string only.
({ status: "approved", riskLevel: null, userAuthorization: null, rationale: 1 }) satisfies GuardianApprovalReview;
// @ts-expect-error snake-case aliases do not replace canonical fields.
({ status: "approved", risk_level: null, userAuthorization: null, rationale: null }) satisfies GuardianApprovalReview;
// @ts-expect-error review records are closed.
({ status: "approved", riskLevel: null, userAuthorization: null, rationale: null, extra: true }) satisfies GuardianApprovalReview;

void (null as unknown as Contracts);
