import type {
  ExternalAgentConfigImportHistoriesReadResponse,
  ExternalAgentConfigImportHistory,
  ExternalAgentConfigImportItemTypeFailure,
  ExternalAgentConfigImportItemTypeSuccess,
  MethodParamsByName,
  MethodResultsByName,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;
type HistoryShape = {
  importId: string;
  completedAtMs: bigint;
  successes: Array<ExternalAgentConfigImportItemTypeSuccess>;
  failures: Array<ExternalAgentConfigImportItemTypeFailure>;
};
type ResponseShape = {
  data: Array<ExternalAgentConfigImportHistory>;
};

type Contracts = [
  Expect<Equal<ExternalAgentConfigImportHistory, HistoryShape>>,
  Expect<Equal<ExternalAgentConfigImportHistoriesReadResponse, ResponseShape>>,
  Expect<Equal<"externalAgentConfig/import" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/import" extends keyof MethodResultsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/import/readHistories" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/import/readHistories" extends keyof MethodResultsByName ? true : false, false>>,
];

export const historyMinimum = {
  importId: "",
  completedAtMs: -9223372036854775808n,
  successes: [],
  failures: [],
} satisfies ExternalAgentConfigImportHistory;

export const responseOrderedDuplicates = {
  data: [
    historyMinimum,
    {
      importId: " import-1 ",
      completedAtMs: 9223372036854775807n,
      successes: [{ itemType: "CONFIG", cwd: null, source: null, target: null }],
      failures: [{ itemType: "HOOKS", errorType: null, failureStage: "", message: "", cwd: null, source: null }],
    },
    historyMinimum,
  ],
} satisfies ExternalAgentConfigImportHistoriesReadResponse;

// @ts-expect-error importId is required.
export const rejectHistoryMissingImportId = { completedAtMs: 0n, successes: [], failures: [] } satisfies ExternalAgentConfigImportHistory;
// @ts-expect-error completedAtMs is required.
export const rejectHistoryMissingTimestamp = { importId: "id", successes: [], failures: [] } satisfies ExternalAgentConfigImportHistory;
// @ts-expect-error successes is required.
export const rejectHistoryMissingSuccesses = { importId: "id", completedAtMs: 0n, failures: [] } satisfies ExternalAgentConfigImportHistory;
// @ts-expect-error failures is required.
export const rejectHistoryMissingFailures = { importId: "id", completedAtMs: 0n, successes: [] } satisfies ExternalAgentConfigImportHistory;
// @ts-expect-error importId is non-null.
export const rejectHistoryNullImportId = { ...historyMinimum, importId: null } satisfies ExternalAgentConfigImportHistory;
// @ts-expect-error completedAtMs is non-null.
export const rejectHistoryNullTimestamp = { ...historyMinimum, completedAtMs: null } satisfies ExternalAgentConfigImportHistory;
// @ts-expect-error the exact ts-rs i64 contract is bigint, not number.
export const rejectHistoryNumberTimestamp = { ...historyMinimum, completedAtMs: 0 } satisfies ExternalAgentConfigImportHistory;
// @ts-expect-error successes is non-null.
export const rejectHistoryNullSuccesses = { ...historyMinimum, successes: null } satisfies ExternalAgentConfigImportHistory;
// @ts-expect-error failures must be an array.
export const rejectHistoryObjectFailures = { ...historyMinimum, failures: {} } satisfies ExternalAgentConfigImportHistory;
// @ts-expect-error nested item types remain closed.
export const rejectHistoryInvalidNested = { ...historyMinimum, successes: [{ itemType: "OTHER", cwd: null, source: null, target: null }] } satisfies ExternalAgentConfigImportHistory;
// @ts-expect-error snake-case aliases do not replace canonical fields.
export const rejectHistorySnakeAlias = { import_id: "id", completed_at_ms: 0n, successes: [], failures: [] } satisfies ExternalAgentConfigImportHistory;
// @ts-expect-error fields absent from the public record are rejected.
export const rejectHistoryExtra = { ...historyMinimum, extra: true } satisfies ExternalAgentConfigImportHistory;

// @ts-expect-error data is required.
export const rejectResponseMissingData = {} satisfies ExternalAgentConfigImportHistoriesReadResponse;
// @ts-expect-error data is non-null.
export const rejectResponseNullData = { data: null } satisfies ExternalAgentConfigImportHistoriesReadResponse;
// @ts-expect-error data must be an array.
export const rejectResponseObjectData = { data: {} } satisfies ExternalAgentConfigImportHistoriesReadResponse;
// @ts-expect-error nested histories retain required failures.
export const rejectResponseInvalidNested = { data: [{ importId: "id", completedAtMs: 0n, successes: [] }] } satisfies ExternalAgentConfigImportHistoriesReadResponse;
// @ts-expect-error snake-case aliases do not replace canonical fields.
export const rejectResponseSnakeAlias = { import_histories: [] } satisfies ExternalAgentConfigImportHistoriesReadResponse;
// @ts-expect-error fields absent from the public record are rejected.
export const rejectResponseExtra = { ...responseOrderedDuplicates, extra: true } satisfies ExternalAgentConfigImportHistoriesReadResponse;

void (null as unknown as Contracts);
