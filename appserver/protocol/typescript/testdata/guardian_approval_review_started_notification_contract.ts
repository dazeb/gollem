import type {
  GuardianApprovalReview,
  GuardianApprovalReviewAction,
  ItemGuardianApprovalReviewStartedNotification,
  MethodParamsByName,
  MethodResultsByName,
} from "../gollem_appserver_protocol.js";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;
type StartedMethod = "item/autoApprovalReview/started";

type Contracts = [
  Expect<Equal<ItemGuardianApprovalReviewStartedNotification, {
    threadId: string;
    turnId: string;
    startedAtMs: number;
    reviewId: string;
    targetItemId: string | null;
    review: GuardianApprovalReview;
    action: GuardianApprovalReviewAction;
  }>>,
  Expect<Equal<Extract<keyof MethodParamsByName, StartedMethod>, never>>,
  Expect<Equal<Extract<keyof MethodResultsByName, StartedMethod>, never>>,
];

const review: GuardianApprovalReview = {
  status: "inProgress",
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
  reviewId: "",
  targetItemId: null,
  review,
  action,
}) satisfies ItemGuardianApprovalReviewStartedNotification;
({
  threadId: "thread",
  turnId: "turn",
  startedAtMs: Number.MAX_SAFE_INTEGER,
  reviewId: "review",
  targetItemId: "item",
  review,
  action,
}) satisfies ItemGuardianApprovalReviewStartedNotification;

// @ts-expect-error threadId is required.
({ turnId: "", startedAtMs: 0, reviewId: "", targetItemId: null, review, action }) satisfies ItemGuardianApprovalReviewStartedNotification;
// @ts-expect-error turnId is required.
({ threadId: "", startedAtMs: 0, reviewId: "", targetItemId: null, review, action }) satisfies ItemGuardianApprovalReviewStartedNotification;
// @ts-expect-error startedAtMs is required.
({ threadId: "", turnId: "", reviewId: "", targetItemId: null, review, action }) satisfies ItemGuardianApprovalReviewStartedNotification;
// @ts-expect-error startedAtMs is a number, not bigint.
({ threadId: "", turnId: "", startedAtMs: 0n, reviewId: "", targetItemId: null, review, action }) satisfies ItemGuardianApprovalReviewStartedNotification;
// @ts-expect-error reviewId is required.
({ threadId: "", turnId: "", startedAtMs: 0, targetItemId: null, review, action }) satisfies ItemGuardianApprovalReviewStartedNotification;
// @ts-expect-error targetItemId is required nullable output.
({ threadId: "", turnId: "", startedAtMs: 0, reviewId: "", review, action }) satisfies ItemGuardianApprovalReviewStartedNotification;
// @ts-expect-error review is required and non-null.
({ threadId: "", turnId: "", startedAtMs: 0, reviewId: "", targetItemId: null, review: null, action }) satisfies ItemGuardianApprovalReviewStartedNotification;
// @ts-expect-error action is required and non-null.
({ threadId: "", turnId: "", startedAtMs: 0, reviewId: "", targetItemId: null, review, action: null }) satisfies ItemGuardianApprovalReviewStartedNotification;
// @ts-expect-error records are closed.
({ threadId: "", turnId: "", startedAtMs: 0, reviewId: "", targetItemId: null, review, action, extra: true }) satisfies ItemGuardianApprovalReviewStartedNotification;

void (null as unknown as Contracts);
