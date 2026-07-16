import type { FileChange } from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type ExactFileChange =
  | { type: "add"; content: string }
  | { type: "delete"; content: string }
  | { type: "update"; unified_diff: string; move_path: string | null };

type Contract = Expect<Equal<FileChange, ExactFileChange>>;

({ type: "add", content: "" }) satisfies FileChange;
({ type: "delete", content: "old" }) satisfies FileChange;
({ type: "update", unified_diff: "", move_path: null }) satisfies FileChange;
({ type: "update", unified_diff: "diff", move_path: "next path" }) satisfies FileChange;

// @ts-expect-error variants are closed.
({ type: "other" }) satisfies FileChange;
// @ts-expect-error tags are case-sensitive.
({ type: "Add", content: "value" }) satisfies FileChange;
// @ts-expect-error add content is required.
({ type: "add" }) satisfies FileChange;
// @ts-expect-error delete content is a string.
({ type: "delete", content: null }) satisfies FileChange;
// @ts-expect-error canonical update move_path is required.
({ type: "update", unified_diff: "diff" }) satisfies FileChange;
// @ts-expect-error update move_path is string or null.
({ type: "update", unified_diff: "diff", move_path: 1 }) satisfies FileChange;
// @ts-expect-error canonical updates use snake_case.
({ type: "update", unified_diff: "diff", movePath: null }) satisfies FileChange;
// @ts-expect-error variants cannot cross fields.
({ type: "add", content: "value", unified_diff: "diff" }) satisfies FileChange;
// @ts-expect-error values are non-null objects.
(null) satisfies FileChange;

void (null as unknown as Contract);
