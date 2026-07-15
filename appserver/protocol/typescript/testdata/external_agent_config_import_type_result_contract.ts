import type {
  ExternalAgentConfigImportItemTypeFailure,
  ExternalAgentConfigImportItemTypeSuccess,
  ExternalAgentConfigImportTypeResult,
  ExternalAgentConfigMigrationItemType,
  MethodParamsByName,
  MethodResultsByName,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type Contracts = [
  Expect<Equal<ExternalAgentConfigImportTypeResult, {
    itemType: ExternalAgentConfigMigrationItemType;
    successes: Array<ExternalAgentConfigImportItemTypeSuccess>;
    failures: Array<ExternalAgentConfigImportItemTypeFailure>;
  }>>,
  Expect<Equal<"externalAgentConfig/import" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/import" extends keyof MethodResultsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/import/readHistories" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/import/readHistories" extends keyof MethodResultsByName ? true : false, false>>,
];

export const empty = {
  itemType: "CONFIG",
  successes: [],
  failures: [],
} satisfies ExternalAgentConfigImportTypeResult;

export const orderedDuplicates = {
  itemType: "HOOKS",
  successes: [
    { itemType: "CONFIG", cwd: null, source: null, target: null },
    { itemType: "SKILLS", cwd: "repo/../repo", source: " Claude Code ", target: "" },
    { itemType: "CONFIG", cwd: null, source: null, target: null },
  ],
  failures: [
    { itemType: "HOOKS", errorType: null, failureStage: "", message: "", cwd: null, source: null },
    { itemType: "HOOKS", errorType: null, failureStage: "", message: "", cwd: null, source: null },
  ],
} satisfies ExternalAgentConfigImportTypeResult;

// @ts-expect-error itemType is required.
export const rejectMissingItemType = { successes: [], failures: [] } satisfies ExternalAgentConfigImportTypeResult;
// @ts-expect-error successes is required.
export const rejectMissingSuccesses = { itemType: "CONFIG", failures: [] } satisfies ExternalAgentConfigImportTypeResult;
// @ts-expect-error failures is required.
export const rejectMissingFailures = { itemType: "CONFIG", successes: [] } satisfies ExternalAgentConfigImportTypeResult;
// @ts-expect-error itemType is non-null.
export const rejectNullItemType = { ...empty, itemType: null } satisfies ExternalAgentConfigImportTypeResult;
// @ts-expect-error itemType is closed.
export const rejectUnknownItemType = { ...empty, itemType: "OTHER" } satisfies ExternalAgentConfigImportTypeResult;
// @ts-expect-error successes is non-null.
export const rejectNullSuccesses = { ...empty, successes: null } satisfies ExternalAgentConfigImportTypeResult;
// @ts-expect-error failures is non-null.
export const rejectNullFailures = { ...empty, failures: null } satisfies ExternalAgentConfigImportTypeResult;
// @ts-expect-error successes must be an array.
export const rejectObjectSuccesses = { ...empty, successes: {} } satisfies ExternalAgentConfigImportTypeResult;
// @ts-expect-error failures must be an array.
export const rejectStringFailures = { ...empty, failures: "failures" } satisfies ExternalAgentConfigImportTypeResult;
// @ts-expect-error nested success retains required target.
export const rejectInvalidSuccess = { ...empty, successes: [{ itemType: "CONFIG", cwd: null, source: null }] } satisfies ExternalAgentConfigImportTypeResult;
// @ts-expect-error nested failure retains required failureStage.
export const rejectInvalidFailure = { ...empty, failures: [{ itemType: "HOOKS", errorType: null, message: "x", cwd: null, source: null }] } satisfies ExternalAgentConfigImportTypeResult;
// @ts-expect-error snake-case aliases do not replace canonical fields.
export const rejectSnakeAlias = { item_type: "CONFIG", successes: [], failures: [] } satisfies ExternalAgentConfigImportTypeResult;
// @ts-expect-error fields absent from the public record are rejected.
export const rejectExtra = { ...empty, extra: true } satisfies ExternalAgentConfigImportTypeResult;

void (null as unknown as Contracts);
