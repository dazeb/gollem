import type {
  MethodParamsByName,
  MethodResultsByName,
  SessionMigration,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type Expected = {
  cwd: string;
  path: string;
  title: string | null;
};
type Contracts = [
  Expect<Equal<SessionMigration, Expected>>,
  Expect<Equal<"externalAgentConfig/detect" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/detect" extends keyof MethodResultsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/import" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/import" extends keyof MethodResultsByName ? true : false, false>>,
];

export const empty = { path: "", cwd: "", title: null } satisfies SessionMigration;
export const relative = { path: "sessions/session.jsonl", cwd: "repo/../repo", title: "" } satisfies SessionMigration;
export const absolute = { path: "/tmp/session.jsonl", cwd: "/tmp/repo", title: "Title" } satisfies SessionMigration;

// @ts-expect-error all canonical fields are required.
export const rejectMissingAll = {} satisfies SessionMigration;
// @ts-expect-error path is required.
export const rejectMissingPath = { cwd: "repo", title: null } satisfies SessionMigration;
// @ts-expect-error cwd is required.
export const rejectMissingCwd = { path: "session", title: null } satisfies SessionMigration;
// @ts-expect-error canonical TypeScript requires explicit nullable title.
export const rejectMissingTitle = { path: "session", cwd: "repo" } satisfies SessionMigration;
// @ts-expect-error path is non-null.
export const rejectNullPath = { path: null, cwd: "repo", title: null } satisfies SessionMigration;
// @ts-expect-error path must be a string.
export const rejectNumberPath = { path: 1, cwd: "repo", title: null } satisfies SessionMigration;
// @ts-expect-error cwd is non-null.
export const rejectNullCwd = { path: "session", cwd: null, title: null } satisfies SessionMigration;
// @ts-expect-error cwd must be a string.
export const rejectNumberCwd = { path: "session", cwd: 1, title: null } satisfies SessionMigration;
// @ts-expect-error title must be string or null.
export const rejectNumberTitle = { path: "session", cwd: "repo", title: 1 } satisfies SessionMigration;
// @ts-expect-error title must be string or null.
export const rejectObjectTitle = { path: "session", cwd: "repo", title: {} } satisfies SessionMigration;
// @ts-expect-error aliases do not replace canonical fields.
export const rejectAliases = { sessionPath: "session", workingDirectory: "repo", title: null } satisfies SessionMigration;
// @ts-expect-error fields absent from the public record are rejected.
export const rejectExtra = { path: "session", cwd: "repo", title: null, extra: true } satisfies SessionMigration;

void (null as unknown as Contracts);
