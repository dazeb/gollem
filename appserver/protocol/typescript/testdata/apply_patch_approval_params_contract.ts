import type {
  ApplyPatchApprovalParams,
  FileChange,
  FileChangeApprovalRequestParams,
  MethodParamsByName,
  MethodResultsByName,
  ThreadId,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;
type ExpectFalse<T extends false> = T;

type ExactParams = {
  conversationId: ThreadId;
  callId: string;
  fileChanges: { [key in string]?: FileChange };
  reason: string | null;
  grantRoot: string | null;
};

export type ApplyPatchApprovalParamsContract = [
  Expect<Equal<ApplyPatchApprovalParams, ExactParams>>,
  ExpectFalse<Equal<ApplyPatchApprovalParams, FileChangeApprovalRequestParams>>,
  ExpectFalse<"applyPatchApproval" extends keyof MethodParamsByName ? true : false>,
  ExpectFalse<"applyPatchApproval" extends keyof MethodResultsByName ? true : false>,
  Expect<Equal<Extract<MethodParamsByName[keyof MethodParamsByName], ApplyPatchApprovalParams>, never>>,
  Expect<Equal<Extract<MethodResultsByName[keyof MethodResultsByName], ApplyPatchApprovalParams>, never>>,
];

({
  conversationId: "",
  callId: "",
  fileChanges: {},
  reason: null,
  grantRoot: null,
}) satisfies ApplyPatchApprovalParams;

({
  conversationId: "thread",
  callId: "call",
  fileChanges: {
    "relative path": { type: "add", content: "" },
    "/absolute": { type: "update", unified_diff: "diff", move_path: null },
  },
  reason: "inspect",
  grantRoot: "relative/root",
}) satisfies ApplyPatchApprovalParams;

// @ts-expect-error conversationId is required.
({ callId: "call", fileChanges: {}, reason: null, grantRoot: null }) satisfies ApplyPatchApprovalParams;
// @ts-expect-error callId is required.
({ conversationId: "thread", fileChanges: {}, reason: null, grantRoot: null }) satisfies ApplyPatchApprovalParams;
// @ts-expect-error fileChanges is required.
({ conversationId: "thread", callId: "call", reason: null, grantRoot: null }) satisfies ApplyPatchApprovalParams;
// @ts-expect-error nested file changes remain strict.
({ conversationId: "thread", callId: "call", fileChanges: { path: { type: "add" } }, reason: null, grantRoot: null }) satisfies ApplyPatchApprovalParams;
// @ts-expect-error reason is canonical-required nullable.
({ conversationId: "thread", callId: "call", fileChanges: {}, grantRoot: null }) satisfies ApplyPatchApprovalParams;
// @ts-expect-error grantRoot is canonical-required nullable.
({ conversationId: "thread", callId: "call", fileChanges: {}, reason: null }) satisfies ApplyPatchApprovalParams;
// @ts-expect-error canonical records are closed.
({ conversationId: "thread", callId: "call", fileChanges: {}, reason: null, grantRoot: null, future: true }) satisfies ApplyPatchApprovalParams;
