import type {
  TurnPlanStep,
  TurnPlanStepStatus,
  TurnPlanUpdatedNotification,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type StepExpected = { step: string; status: TurnPlanStepStatus };
type NotificationExpected = {
  threadId: string;
  turnId: string;
  explanation: string | null;
  plan: TurnPlanStep[];
};
type Contracts = [
  Expect<Equal<TurnPlanStepStatus, "pending" | "inProgress" | "completed">>,
  Expect<Equal<TurnPlanStep, StepExpected>>,
  Expect<Equal<TurnPlanUpdatedNotification, NotificationExpected>>,
];

export const statuses = ["pending", "inProgress", "completed"] as const satisfies readonly TurnPlanStepStatus[];
export const empty = {
  threadId: "",
  turnId: "",
  explanation: null,
  plan: [],
} satisfies TurnPlanUpdatedNotification;
export const populated = {
  threadId: "thread",
  turnId: "turn",
  explanation: "Ship safely",
  plan: [
    { step: "Inspect", status: "pending" },
    { step: "Implement", status: "inProgress" },
    { step: "Verify", status: "completed" },
  ],
} satisfies TurnPlanUpdatedNotification;

// @ts-expect-error statuses are closed.
export const rejectStatus = "in_progress" satisfies TurnPlanStepStatus;
// @ts-expect-error step text is required.
export const rejectMissingStep = { status: "pending" } satisfies TurnPlanStep;
// @ts-expect-error step status is required.
export const rejectMissingStatus = { step: "Inspect" } satisfies TurnPlanStep;
// @ts-expect-error threadId is required.
export const rejectMissingThread = { turnId: "turn", explanation: null, plan: [] } satisfies TurnPlanUpdatedNotification;
// @ts-expect-error turnId is required.
export const rejectMissingTurn = { threadId: "thread", explanation: null, plan: [] } satisfies TurnPlanUpdatedNotification;
// @ts-expect-error canonical TypeScript requires explicit nullable explanation.
export const rejectMissingExplanation = { threadId: "thread", turnId: "turn", plan: [] } satisfies TurnPlanUpdatedNotification;
// @ts-expect-error plan is required.
export const rejectMissingPlan = { threadId: "thread", turnId: "turn", explanation: null } satisfies TurnPlanUpdatedNotification;
// @ts-expect-error plan is non-null.
export const rejectNullPlan = { threadId: "thread", turnId: "turn", explanation: null, plan: null } satisfies TurnPlanUpdatedNotification;
// @ts-expect-error nested steps remain strict.
export const rejectMalformedStep = { threadId: "thread", turnId: "turn", explanation: null, plan: [{ step: "Inspect" }] } satisfies TurnPlanUpdatedNotification;
// @ts-expect-error explanation is nullable string only.
export const rejectNumericExplanation = { threadId: "thread", turnId: "turn", explanation: 1, plan: [] } satisfies TurnPlanUpdatedNotification;
// @ts-expect-error notification objects are closed.
export const rejectUnknown = { threadId: "thread", turnId: "turn", explanation: null, plan: [], steps: [] } satisfies TurnPlanUpdatedNotification;

void (null as unknown as Contracts);
