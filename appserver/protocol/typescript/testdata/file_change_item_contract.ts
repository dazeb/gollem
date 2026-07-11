import type {
  FileUpdateChange,
  PatchApplyStatus,
  PatchChangeKind,
} from "../gollem_appserver_protocol";

export const publicUpdate = {
  type: "update",
  move_path: null,
} satisfies PatchChangeKind;
export const legacyUpdate = {
  type: "update",
  movePath: "renamed.txt",
} satisfies PatchChangeKind;
export const canonicalCompatibleUpdate = {
  type: "update",
  move_path: "renamed.txt",
  movePath: "renamed.txt",
} satisfies PatchChangeKind;
export const fileUpdate = {
  path: "notes.txt",
  kind: publicUpdate,
  diff: "@@ -1 +1 @@\n-old\n+new\n",
} satisfies FileUpdateChange;

export const statuses = [
  "inProgress",
  "completed",
  "failed",
  "declined",
] as const satisfies readonly PatchApplyStatus[];

// @ts-expect-error update requires public move_path or legacy movePath.
export const rejectUpdateWithoutMovePath = { type: "update" } satisfies PatchChangeKind;
// @ts-expect-error add does not carry update-only move fields.
export const rejectAddMovePath = { type: "add", move_path: null } satisfies PatchChangeKind;
// @ts-expect-error delete does not carry update-only move fields.
export const rejectDeleteLegacyMovePath = { type: "delete", movePath: null } satisfies PatchChangeKind;
// @ts-expect-error patch kinds are a closed discriminated union.
export const rejectUnknownKind = { type: "move", move_path: null } satisfies PatchChangeKind;
// @ts-expect-error patch status is a closed public enum.
export const rejectUnknownStatus = "cancelled" satisfies PatchApplyStatus;
