import type {
  AbsolutePathBuf,
  CommandAction,
} from "../gollem_appserver_protocol";

const readPath = "/workspace/file" satisfies AbsolutePathBuf;
export const actions = [
  { type: "read", command: "cat file", name: "cat", path: readPath },
  { type: "listFiles", command: "ls", path: null },
  { type: "listFiles", command: "ls src", path: "relative/src" },
  { type: "search", command: "rg needle", query: null, path: null },
  { type: "search", command: "rg needle src", query: "needle", path: "relative/src" },
  { type: "unknown", command: "custom --flag" },
] satisfies CommandAction[];

// @ts-expect-error read requires a path.
export const rejectReadWithoutPath = { type: "read", command: "cat", name: "cat" } satisfies CommandAction;
// @ts-expect-error read path cannot be null.
export const rejectNullReadPath = { type: "read", command: "cat", name: "cat", path: null } satisfies CommandAction;
// @ts-expect-error listFiles path is required nullable, not optional.
export const rejectListWithoutPath = { type: "listFiles", command: "ls" } satisfies CommandAction;
// @ts-expect-error search query is required nullable, not optional.
export const rejectSearchWithoutQuery = { type: "search", command: "rg", path: null } satisfies CommandAction;
// @ts-expect-error search path is required nullable, not optional.
export const rejectSearchWithoutPath = { type: "search", command: "rg", query: null } satisfies CommandAction;
// @ts-expect-error command-action variants cannot cross fields.
export const rejectCrossedAction = { type: "unknown", command: "run", path: null } satisfies CommandAction;
// @ts-expect-error command-action discriminators are closed.
export const rejectUnknownType = { type: "execute", command: "run" } satisfies CommandAction;
