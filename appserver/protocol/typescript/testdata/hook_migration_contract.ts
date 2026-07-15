import type {
  HookMigration,
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
  Expect<Equal<HookMigration, Expected>>,
  Expect<Equal<"externalAgentConfig/detect" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/detect" extends keyof MethodResultsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/import" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/import" extends keyof MethodResultsByName ? true : false, false>>,
];

export const empty = { name: "" } satisfies HookMigration;
export const named = { name: "hook" } satisfies HookMigration;

// @ts-expect-error name is required.
export const rejectMissingName = {} satisfies HookMigration;
// @ts-expect-error name is non-null.
export const rejectNullName = { name: null } satisfies HookMigration;
// @ts-expect-error name must be a string.
export const rejectNumberName = { name: 1 } satisfies HookMigration;
// @ts-expect-error name must be a string.
export const rejectBooleanName = { name: true } satisfies HookMigration;
// @ts-expect-error name must be a string.
export const rejectObjectName = { name: {} } satisfies HookMigration;
// @ts-expect-error name must be a string.
export const rejectArrayName = { name: [] } satisfies HookMigration;
// @ts-expect-error aliases do not replace the canonical name field.
export const rejectCamelAlias = { hookName: "hook" } satisfies HookMigration;
// @ts-expect-error snake-case aliases do not replace the canonical name field.
export const rejectSnakeAlias = { hook_name: "hook" } satisfies HookMigration;
// @ts-expect-error fields absent from the public record are rejected.
export const rejectExtra = { name: "hook", extra: true } satisfies HookMigration;

void (null as unknown as Contracts);
