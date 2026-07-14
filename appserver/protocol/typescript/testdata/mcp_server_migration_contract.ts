import type {
  McpServerMigration,
  MethodParamsByName,
  MethodResultsByName,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type Expected = {
  name: string;
};
type Contracts = [
  Expect<Equal<McpServerMigration, Expected>>,
  Expect<Equal<"externalAgentConfig/detect" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/detect" extends keyof MethodResultsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/import" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/import" extends keyof MethodResultsByName ? true : false, false>>,
];

export const empty = { name: "" } satisfies McpServerMigration;
export const named = { name: "server" } satisfies McpServerMigration;

// @ts-expect-error name is required.
export const rejectMissingName = {} satisfies McpServerMigration;
// @ts-expect-error name is non-null.
export const rejectNullName = { name: null } satisfies McpServerMigration;
// @ts-expect-error name must be a string.
export const rejectNumberName = { name: 1 } satisfies McpServerMigration;
// @ts-expect-error name must be a string.
export const rejectBooleanName = { name: true } satisfies McpServerMigration;
// @ts-expect-error name must be a string.
export const rejectObjectName = { name: {} } satisfies McpServerMigration;
// @ts-expect-error name must be a string.
export const rejectArrayName = { name: [] } satisfies McpServerMigration;
// @ts-expect-error aliases do not replace the canonical name field.
export const rejectCamelAlias = { serverName: "server" } satisfies McpServerMigration;
// @ts-expect-error snake-case aliases do not replace the canonical name field.
export const rejectSnakeAlias = { server_name: "server" } satisfies McpServerMigration;
// @ts-expect-error fields absent from the public record are rejected.
export const rejectExtra = { name: "server", extra: true } satisfies McpServerMigration;

void (null as unknown as Contracts);
