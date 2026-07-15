import type {
  MethodParamsByName,
  MethodResultsByName,
  SubagentMigration,
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
  Expect<Equal<SubagentMigration, Expected>>,
  Expect<Equal<"externalAgentConfig/detect" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/detect" extends keyof MethodResultsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/import" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/import" extends keyof MethodResultsByName ? true : false, false>>,
];

export const empty = { name: "" } satisfies SubagentMigration;
export const named = { name: "subagent" } satisfies SubagentMigration;

// @ts-expect-error name is required.
export const rejectMissingName = {} satisfies SubagentMigration;
// @ts-expect-error name is non-null.
export const rejectNullName = { name: null } satisfies SubagentMigration;
// @ts-expect-error name must be a string.
export const rejectNumberName = { name: 1 } satisfies SubagentMigration;
// @ts-expect-error name must be a string.
export const rejectBooleanName = { name: true } satisfies SubagentMigration;
// @ts-expect-error name must be a string.
export const rejectObjectName = { name: {} } satisfies SubagentMigration;
// @ts-expect-error name must be a string.
export const rejectArrayName = { name: [] } satisfies SubagentMigration;
// @ts-expect-error aliases do not replace the canonical name field.
export const rejectCamelAlias = { agentName: "subagent" } satisfies SubagentMigration;
// @ts-expect-error snake-case aliases do not replace the canonical name field.
export const rejectSnakeAlias = { agent_name: "subagent" } satisfies SubagentMigration;
// @ts-expect-error fields absent from the public record are rejected.
export const rejectExtra = { name: "subagent", extra: true } satisfies SubagentMigration;

void (null as unknown as Contracts);
