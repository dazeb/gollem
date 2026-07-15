import type {
  MethodParamsByName,
  MethodResultsByName,
  CommandMigration,
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
  Expect<Equal<CommandMigration, Expected>>,
  Expect<Equal<"externalAgentConfig/detect" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/detect" extends keyof MethodResultsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/import" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/import" extends keyof MethodResultsByName ? true : false, false>>,
];

export const empty = { name: "" } satisfies CommandMigration;
export const named = { name: "command" } satisfies CommandMigration;

// @ts-expect-error name is required.
export const rejectMissingName = {} satisfies CommandMigration;
// @ts-expect-error name is non-null.
export const rejectNullName = { name: null } satisfies CommandMigration;
// @ts-expect-error name must be a string.
export const rejectNumberName = { name: 1 } satisfies CommandMigration;
// @ts-expect-error name must be a string.
export const rejectBooleanName = { name: true } satisfies CommandMigration;
// @ts-expect-error name must be a string.
export const rejectObjectName = { name: {} } satisfies CommandMigration;
// @ts-expect-error name must be a string.
export const rejectArrayName = { name: [] } satisfies CommandMigration;
// @ts-expect-error aliases do not replace the canonical name field.
export const rejectCamelAlias = { commandName: "command" } satisfies CommandMigration;
// @ts-expect-error snake-case aliases do not replace the canonical name field.
export const rejectSnakeAlias = { command_name: "command" } satisfies CommandMigration;
// @ts-expect-error fields absent from the public record are rejected.
export const rejectExtra = { name: "command", extra: true } satisfies CommandMigration;

void (null as unknown as Contracts);
