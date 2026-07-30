import type {
  MethodParamsByName,
  MethodResultsByName,
  ReviewDelivery,
  ReviewStartParams,
  ReviewStartResponse,
  ReviewTarget,
  Turn,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2) ? true : false;
type Expect<T extends true> = T;

type Contracts = [
  Expect<Equal<ReviewDelivery, "inline" | "detached">>,
  Expect<Equal<ReviewTarget,
    | { type: "baseBranch"; branch: string }
    | { type: "commit"; sha: string; title: string | null }
    | { type: "custom"; instructions: string }
    | { type: "uncommittedChanges" }
  >>,
  Expect<Equal<ReviewStartParams, {
    delivery?: ReviewDelivery | null;
    target: ReviewTarget;
    threadId: string;
  }>>,
  Expect<Equal<ReviewStartResponse, {
    reviewThreadId: string;
    turn: Turn;
  }>>,
  Expect<Equal<"review/start" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"review/start" extends keyof MethodResultsByName ? true : false, false>>,
];

({ type: "uncommittedChanges" }) satisfies ReviewTarget;
({ type: "baseBranch", branch: "main" }) satisfies ReviewTarget;
({ type: "commit", sha: "deadbeef", title: null }) satisfies ReviewTarget;
({ type: "custom", instructions: "Review this." }) satisfies ReviewTarget;
({ threadId: "thread", target: { type: "uncommittedChanges" } }) satisfies ReviewStartParams;

// @ts-expect-error branch is required for a base-branch target.
({ type: "baseBranch" }) satisfies ReviewTarget;
// @ts-expect-error title is an explicit nullable value in the public TypeScript contract.
({ type: "commit", sha: "deadbeef" }) satisfies ReviewTarget;
// @ts-expect-error target is required.
({ threadId: "thread" }) satisfies ReviewStartParams;

void (null as unknown as Contracts);
