import type { AutoReviewDecisionSource } from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? (<T>() => T extends B ? 1 : 2) extends
        (<T>() => T extends A ? 1 : 2)
      ? true
      : false
    : false;
type Expect<T extends true> = T;

type Contract = Expect<Equal<AutoReviewDecisionSource, "agent">>;

export const source = "agent" satisfies AutoReviewDecisionSource;

// @ts-expect-error decision sources are closed.
export const rejectUnknown = "other" satisfies AutoReviewDecisionSource;
// @ts-expect-error exact lowercase spelling is required.
export const rejectCase = "Agent" satisfies AutoReviewDecisionSource;
// @ts-expect-error empty strings are not decision sources.
export const rejectEmpty = "" satisfies AutoReviewDecisionSource;
// @ts-expect-error decision sources are non-null.
export const rejectNull = null satisfies AutoReviewDecisionSource;
// @ts-expect-error decision sources are strings.
export const rejectNumber = 1 satisfies AutoReviewDecisionSource;

void (null as unknown as Contract);
