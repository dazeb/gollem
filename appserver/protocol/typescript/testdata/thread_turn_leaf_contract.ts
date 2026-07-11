import type {
  GitInfo,
  NonSteerableTurnKind,
  ThreadActiveFlag,
  ThreadId,
  ThreadSource,
  TurnItemsView,
  TurnStatus,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type ThreadIdIsExact = Expect<Equal<ThreadId, string>>;
type NonSteerableTurnKindIsExact = Expect<Equal<NonSteerableTurnKind, "review" | "compact">>;
type TurnItemsViewIsExact = Expect<Equal<TurnItemsView, "notLoaded" | "summary" | "full">>;
type TurnStatusIsExact = Expect<Equal<TurnStatus, "completed" | "interrupted" | "failed" | "inProgress">>;
type ThreadActiveFlagIsExact = Expect<Equal<ThreadActiveFlag, "waitingOnApproval" | "waitingOnUserInput">>;
type ThreadSourceIsExact = Expect<Equal<ThreadSource, string>>;
type GitInfoIsExact = Expect<
  Equal<GitInfo, { branch: string | null; originUrl: string | null; sha: string | null }>
>;

export const threadIds = ["", "thread-1", "not-a-uuid"] satisfies ThreadId[];
export const nonSteerableKinds = ["review", "compact"] satisfies NonSteerableTurnKind[];
export const itemViews = ["notLoaded", "summary", "full"] satisfies TurnItemsView[];
export const turnStatuses = ["completed", "interrupted", "failed", "inProgress"] satisfies TurnStatus[];
export const activeFlags = ["waitingOnApproval", "waitingOnUserInput"] satisfies ThreadActiveFlag[];
export const threadSources = ["", "user", "feature/custom"] satisfies ThreadSource[];
export const emptyGitInfo = { sha: null, branch: null, originUrl: null } satisfies GitInfo;
export const populatedGitInfo = { sha: "abc", branch: "main", originUrl: "origin" } satisfies GitInfo;

// @ts-expect-error non-steerable turn kinds are closed.
export const rejectUnknownKind = "other" satisfies NonSteerableTurnKind;
// @ts-expect-error turn item views use exact camel-case spelling.
export const rejectWrongCaseView = "notloaded" satisfies TurnItemsView;
// @ts-expect-error public turn status uses inProgress, not Gollem's durable running value.
export const rejectDurableTurnStatus = "running" satisfies TurnStatus;
// @ts-expect-error thread active flags are closed.
export const rejectUnknownActiveFlag = "other" satisfies ThreadActiveFlag;
// @ts-expect-error GitInfo requires sha even when its value is null.
export const rejectMissingSha = { branch: null, originUrl: null } satisfies GitInfo;
// @ts-expect-error GitInfo requires branch even when its value is null.
export const rejectMissingBranch = { sha: null, originUrl: null } satisfies GitInfo;
// @ts-expect-error GitInfo requires originUrl even when its value is null.
export const rejectMissingOrigin = { sha: null, branch: null } satisfies GitInfo;
// @ts-expect-error GitInfo fields are nullable strings, not numbers.
export const rejectNumericSha = { sha: 1, branch: null, originUrl: null } satisfies GitInfo;
// @ts-expect-error GitInfo is a closed object for literal consumers.
export const rejectExtraGitField = { sha: null, branch: null, originUrl: null, extra: true } satisfies GitInfo;

declare const threadIdIsExact: ThreadIdIsExact;
declare const nonSteerableTurnKindIsExact: NonSteerableTurnKindIsExact;
declare const turnItemsViewIsExact: TurnItemsViewIsExact;
declare const turnStatusIsExact: TurnStatusIsExact;
declare const threadActiveFlagIsExact: ThreadActiveFlagIsExact;
declare const threadSourceIsExact: ThreadSourceIsExact;
declare const gitInfoIsExact: GitInfoIsExact;
void threadIdIsExact;
void nonSteerableTurnKindIsExact;
void turnItemsViewIsExact;
void turnStatusIsExact;
void threadActiveFlagIsExact;
void threadSourceIsExact;
void gitInfoIsExact;
