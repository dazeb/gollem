import type {
  ExternalAgentConfigImportParams,
  ExternalAgentConfigImportResponse,
  ExternalAgentConfigMigrationItem,
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
  Expect<Equal<ExternalAgentConfigImportParams, {
    migrationItems: Array<ExternalAgentConfigMigrationItem>;
    source?: string | null;
  }>>,
  Expect<Equal<ExternalAgentConfigImportResponse, { importId: string }>>,
  Expect<Equal<"externalAgentConfig/detect" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/detect" extends keyof MethodResultsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/import" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/import" extends keyof MethodResultsByName ? true : false, false>>,
];

export const item = {
  itemType: "CONFIG",
  description: "config",
  cwd: null,
  details: null,
} satisfies ExternalAgentConfigMigrationItem;
export const empty = { migrationItems: [] } satisfies ExternalAgentConfigImportParams;
export const nullSource = { migrationItems: [item, item], source: null } satisfies ExternalAgentConfigImportParams;
export const namedSource = { migrationItems: [item], source: " Claude Code " } satisfies ExternalAgentConfigImportParams;
export const response = { importId: "" } satisfies ExternalAgentConfigImportResponse;

// @ts-expect-error migrationItems is required.
export const rejectMissingItems = {} satisfies ExternalAgentConfigImportParams;
// @ts-expect-error migrationItems is non-null.
export const rejectNullItems = { migrationItems: null } satisfies ExternalAgentConfigImportParams;
// @ts-expect-error migrationItems is an array.
export const rejectScalarItems = { migrationItems: item } satisfies ExternalAgentConfigImportParams;
// @ts-expect-error migration items retain strict nullable fields.
export const rejectMalformedItem = { migrationItems: [{ itemType: "CONFIG", description: "config" }] } satisfies ExternalAgentConfigImportParams;
// @ts-expect-error source is a nullable string.
export const rejectNumericSource = { migrationItems: [], source: 1 } satisfies ExternalAgentConfigImportParams;
// @ts-expect-error optional fields do not accept explicit undefined.
export const rejectUndefinedSource = { migrationItems: [], source: undefined } satisfies ExternalAgentConfigImportParams;
// @ts-expect-error snake-case aliases do not replace canonical fields.
export const rejectSnakeAlias = { migration_items: [] } satisfies ExternalAgentConfigImportParams;
// @ts-expect-error fields absent from the public request are rejected.
export const rejectParamsExtra = { migrationItems: [], extra: true } satisfies ExternalAgentConfigImportParams;

// @ts-expect-error importId is required.
export const rejectMissingImportId = {} satisfies ExternalAgentConfigImportResponse;
// @ts-expect-error importId is non-null.
export const rejectNullImportId = { importId: null } satisfies ExternalAgentConfigImportResponse;
// @ts-expect-error importId is a string.
export const rejectNumericImportId = { importId: 1 } satisfies ExternalAgentConfigImportResponse;
// @ts-expect-error snake-case aliases do not replace canonical fields.
export const rejectImportIdAlias = { import_id: "import-1" } satisfies ExternalAgentConfigImportResponse;
// @ts-expect-error fields absent from the public response are rejected.
export const rejectResponseExtra = { importId: "import-1", extra: true } satisfies ExternalAgentConfigImportResponse;

void (null as unknown as Contracts);
