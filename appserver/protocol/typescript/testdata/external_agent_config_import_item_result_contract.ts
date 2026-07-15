import type {
  ExternalAgentConfigImportItemTypeFailure,
  ExternalAgentConfigImportItemTypeSuccess,
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
  Expect<Equal<ExternalAgentConfigImportItemTypeSuccess, {
    itemType: ExternalAgentConfigMigrationItemType;
    cwd: string | null;
    source: string | null;
    target: string | null;
  }>>,
  Expect<Equal<ExternalAgentConfigImportItemTypeFailure, {
    itemType: ExternalAgentConfigMigrationItemType;
    errorType: string | null;
    failureStage: string;
    message: string;
    cwd: string | null;
    source: string | null;
  }>>,
  Expect<Equal<"externalAgentConfig/import" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/import" extends keyof MethodResultsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/import/readHistories" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/import/readHistories" extends keyof MethodResultsByName ? true : false, false>>,
];

export const success = {
  itemType: "CONFIG",
  cwd: null,
  source: " Claude Code ",
  target: "",
} satisfies ExternalAgentConfigImportItemTypeSuccess;

export const failure = {
  itemType: "HOOKS",
  errorType: null,
  failureStage: "",
  message: "",
  cwd: "repo/../repo",
  source: null,
} satisfies ExternalAgentConfigImportItemTypeFailure;

// @ts-expect-error itemType is required.
export const rejectSuccessMissingItemType = { cwd: null, source: null, target: null } satisfies ExternalAgentConfigImportItemTypeSuccess;
// @ts-expect-error cwd remains required-nullable in generated TypeScript.
export const rejectSuccessMissingCwd = { itemType: "CONFIG", source: null, target: null } satisfies ExternalAgentConfigImportItemTypeSuccess;
// @ts-expect-error source remains required-nullable in generated TypeScript.
export const rejectSuccessMissingSource = { itemType: "CONFIG", cwd: null, target: null } satisfies ExternalAgentConfigImportItemTypeSuccess;
// @ts-expect-error target remains required-nullable in generated TypeScript.
export const rejectSuccessMissingTarget = { itemType: "CONFIG", cwd: null, source: null } satisfies ExternalAgentConfigImportItemTypeSuccess;
// @ts-expect-error itemType is closed.
export const rejectSuccessUnknownItemType = { ...success, itemType: "OTHER" } satisfies ExternalAgentConfigImportItemTypeSuccess;
// @ts-expect-error cwd is a nullable string.
export const rejectSuccessNumericCwd = { ...success, cwd: 1 } satisfies ExternalAgentConfigImportItemTypeSuccess;
// @ts-expect-error source is a nullable string.
export const rejectSuccessNumericSource = { ...success, source: 1 } satisfies ExternalAgentConfigImportItemTypeSuccess;
// @ts-expect-error target is a nullable string.
export const rejectSuccessNumericTarget = { ...success, target: 1 } satisfies ExternalAgentConfigImportItemTypeSuccess;
// @ts-expect-error snake-case aliases do not replace canonical fields.
export const rejectSuccessSnakeAlias = { item_type: "CONFIG", cwd: null, source: null, target: null } satisfies ExternalAgentConfigImportItemTypeSuccess;
// @ts-expect-error fields absent from the public record are rejected.
export const rejectSuccessExtra = { ...success, extra: true } satisfies ExternalAgentConfigImportItemTypeSuccess;

// @ts-expect-error itemType is required.
export const rejectFailureMissingItemType = { ...failure, itemType: undefined } satisfies ExternalAgentConfigImportItemTypeFailure;
// @ts-expect-error failureStage is required.
export const rejectFailureMissingStage = { itemType: "CONFIG", errorType: null, message: "x", cwd: null, source: null } satisfies ExternalAgentConfigImportItemTypeFailure;
// @ts-expect-error message is required.
export const rejectFailureMissingMessage = { itemType: "CONFIG", errorType: null, failureStage: "x", cwd: null, source: null } satisfies ExternalAgentConfigImportItemTypeFailure;
// @ts-expect-error errorType remains required-nullable in generated TypeScript.
export const rejectFailureMissingErrorType = { itemType: "CONFIG", failureStage: "x", message: "x", cwd: null, source: null } satisfies ExternalAgentConfigImportItemTypeFailure;
// @ts-expect-error cwd remains required-nullable in generated TypeScript.
export const rejectFailureMissingCwd = { itemType: "CONFIG", errorType: null, failureStage: "x", message: "x", source: null } satisfies ExternalAgentConfigImportItemTypeFailure;
// @ts-expect-error source remains required-nullable in generated TypeScript.
export const rejectFailureMissingSource = { itemType: "CONFIG", errorType: null, failureStage: "x", message: "x", cwd: null } satisfies ExternalAgentConfigImportItemTypeFailure;
// @ts-expect-error failureStage is non-null.
export const rejectFailureNullStage = { ...failure, failureStage: null } satisfies ExternalAgentConfigImportItemTypeFailure;
// @ts-expect-error message is non-null.
export const rejectFailureNullMessage = { ...failure, message: null } satisfies ExternalAgentConfigImportItemTypeFailure;
// @ts-expect-error itemType is closed.
export const rejectFailureUnknownItemType = { ...failure, itemType: "OTHER" } satisfies ExternalAgentConfigImportItemTypeFailure;
// @ts-expect-error errorType is a nullable string.
export const rejectFailureNumericErrorType = { ...failure, errorType: 1 } satisfies ExternalAgentConfigImportItemTypeFailure;
// @ts-expect-error fields absent from the public record are rejected.
export const rejectFailureExtra = { ...failure, extra: true } satisfies ExternalAgentConfigImportItemTypeFailure;

void (null as unknown as Contracts);
