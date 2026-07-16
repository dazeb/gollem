import type {
  AutoReviewDecisionSource,
  GuardianApprovalReview,
  GuardianApprovalReviewAction,
  ItemGuardianApprovalReviewCompletedNotification,
  MethodParamsByName,
  MethodResultsByName,
} from "../gollem_appserver_protocol.js";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;
type GuardianNotificationMethod =
  | "item/autoApprovalReview/started"
  | "item/autoApprovalReview/completed";

type Contracts = [
  Expect<Equal<ItemGuardianApprovalReviewCompletedNotification, {
    threadId: string;
    turnId: string;
    startedAtMs: number;
    completedAtMs: number;
    reviewId: string;
    targetItemId: string | null;
    decisionSource: AutoReviewDecisionSource;
    review: GuardianApprovalReview;
    action: GuardianApprovalReviewAction;
  }>>,
  Expect<Equal<Extract<keyof MethodParamsByName, GuardianNotificationMethod>, never>>,
  Expect<Equal<Extract<keyof MethodResultsByName, GuardianNotificationMethod>, never>>,
];

const review: GuardianApprovalReview = {
  status: "approved",
  riskLevel: null,
  userAuthorization: null,
  rationale: null,
};
const action: GuardianApprovalReviewAction = {
  type: "command",
  source: "shell",
  command: "",
  cwd: "/workspace",
};

({
  threadId: "",
  turnId: "",
  startedAtMs: Number.MIN_SAFE_INTEGER,
  completedAtMs: Number.MAX_SAFE_INTEGER,
  reviewId: "",
  targetItemId: null,
  decisionSource: "agent",
  review,
  action,
}) satisfies ItemGuardianApprovalReviewCompletedNotification;
({
  threadId: "thread",
  turnId: "turn",
  startedAtMs: 2,
  completedAtMs: 1,
  reviewId: "review",
  targetItemId: "item",
  decisionSource: "agent",
  review,
  action,
}) satisfies ItemGuardianApprovalReviewCompletedNotification;

// @ts-expect-error threadId is required.
({ turnId: "", startedAtMs: 0, completedAtMs: 0, reviewId: "", targetItemId: null, decisionSource: "agent", review, action }) satisfies ItemGuardianApprovalReviewCompletedNotification;
// @ts-expect-error turnId is required.
({ threadId: "", startedAtMs: 0, completedAtMs: 0, reviewId: "", targetItemId: null, decisionSource: "agent", review, action }) satisfies ItemGuardianApprovalReviewCompletedNotification;
// @ts-expect-error startedAtMs is required.
({ threadId: "", turnId: "", completedAtMs: 0, reviewId: "", targetItemId: null, decisionSource: "agent", review, action }) satisfies ItemGuardianApprovalReviewCompletedNotification;
// @ts-expect-error completedAtMs is required.
({ threadId: "", turnId: "", startedAtMs: 0, reviewId: "", targetItemId: null, decisionSource: "agent", review, action }) satisfies ItemGuardianApprovalReviewCompletedNotification;
// @ts-expect-error timestamps are numbers, not bigint.
({ threadId: "", turnId: "", startedAtMs: 0n, completedAtMs: 0n, reviewId: "", targetItemId: null, decisionSource: "agent", review, action }) satisfies ItemGuardianApprovalReviewCompletedNotification;
// @ts-expect-error reviewId is required.
({ threadId: "", turnId: "", startedAtMs: 0, completedAtMs: 0, targetItemId: null, decisionSource: "agent", review, action }) satisfies ItemGuardianApprovalReviewCompletedNotification;
// @ts-expect-error targetItemId is required nullable output.
({ threadId: "", turnId: "", startedAtMs: 0, completedAtMs: 0, reviewId: "", decisionSource: "agent", review, action }) satisfies ItemGuardianApprovalReviewCompletedNotification;
// @ts-expect-error decisionSource is required and closed.
({ threadId: "", turnId: "", startedAtMs: 0, completedAtMs: 0, reviewId: "", targetItemId: null, decisionSource: "other", review, action }) satisfies ItemGuardianApprovalReviewCompletedNotification;
// @ts-expect-error review is required and non-null.
({ threadId: "", turnId: "", startedAtMs: 0, completedAtMs: 0, reviewId: "", targetItemId: null, decisionSource: "agent", review: null, action }) satisfies ItemGuardianApprovalReviewCompletedNotification;
// @ts-expect-error action is required and non-null.
({ threadId: "", turnId: "", startedAtMs: 0, completedAtMs: 0, reviewId: "", targetItemId: null, decisionSource: "agent", review, action: null }) satisfies ItemGuardianApprovalReviewCompletedNotification;
// @ts-expect-error records are closed.
({ threadId: "", turnId: "", startedAtMs: 0, completedAtMs: 0, reviewId: "", targetItemId: null, decisionSource: "agent", review, action, extra: true }) satisfies ItemGuardianApprovalReviewCompletedNotification;

void (null as unknown as Contracts);
