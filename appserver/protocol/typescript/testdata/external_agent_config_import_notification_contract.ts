import type {
  ExternalAgentConfigImportCompletedNotification,
  ExternalAgentConfigImportProgressNotification,
  ExternalAgentConfigImportTypeResult,
  MethodParamsByName,
  MethodResultsByName,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;
type NotificationShape = {
  importId: string;
  itemTypeResults: Array<ExternalAgentConfigImportTypeResult>;
};

type Contracts = [
  Expect<Equal<ExternalAgentConfigImportProgressNotification, NotificationShape>>,
  Expect<Equal<ExternalAgentConfigImportCompletedNotification, NotificationShape>>,
  Expect<Equal<ExternalAgentConfigImportProgressNotification, ExternalAgentConfigImportCompletedNotification>>,
  Expect<Equal<"externalAgentConfig/import" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/import" extends keyof MethodResultsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/import/readHistories" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/import/readHistories" extends keyof MethodResultsByName ? true : false, false>>,
];

export const progressEmpty = {
  importId: "",
  itemTypeResults: [],
} satisfies ExternalAgentConfigImportProgressNotification;

export const completedOrderedDuplicates = {
  importId: " import-1 ",
  itemTypeResults: [
    { itemType: "CONFIG", successes: [], failures: [] },
    { itemType: "HOOKS", successes: [], failures: [] },
    { itemType: "CONFIG", successes: [], failures: [] },
  ],
} satisfies ExternalAgentConfigImportCompletedNotification;

// @ts-expect-error importId is required.
export const rejectProgressMissingImportId = { itemTypeResults: [] } satisfies ExternalAgentConfigImportProgressNotification;
// @ts-expect-error itemTypeResults is required.
export const rejectProgressMissingResults = { importId: "id" } satisfies ExternalAgentConfigImportProgressNotification;
// @ts-expect-error importId is non-null.
export const rejectProgressNullImportId = { ...progressEmpty, importId: null } satisfies ExternalAgentConfigImportProgressNotification;
// @ts-expect-error itemTypeResults is non-null.
export const rejectProgressNullResults = { ...progressEmpty, itemTypeResults: null } satisfies ExternalAgentConfigImportProgressNotification;
// @ts-expect-error itemTypeResults must be an array.
export const rejectProgressObjectResults = { ...progressEmpty, itemTypeResults: {} } satisfies ExternalAgentConfigImportProgressNotification;
// @ts-expect-error nested results retain required failures.
export const rejectProgressInvalidNested = { ...progressEmpty, itemTypeResults: [{ itemType: "CONFIG", successes: [] }] } satisfies ExternalAgentConfigImportProgressNotification;
// @ts-expect-error snake-case aliases do not replace canonical fields.
export const rejectProgressSnakeAlias = { import_id: "id", item_type_results: [] } satisfies ExternalAgentConfigImportProgressNotification;
// @ts-expect-error fields absent from the public record are rejected.
export const rejectProgressExtra = { ...progressEmpty, extra: true } satisfies ExternalAgentConfigImportProgressNotification;

// @ts-expect-error importId is required.
export const rejectCompletedMissingImportId = { itemTypeResults: [] } satisfies ExternalAgentConfigImportCompletedNotification;
// @ts-expect-error itemTypeResults is required.
export const rejectCompletedMissingResults = { importId: "id" } satisfies ExternalAgentConfigImportCompletedNotification;
// @ts-expect-error importId is non-null.
export const rejectCompletedNullImportId = { importId: null, itemTypeResults: [] } satisfies ExternalAgentConfigImportCompletedNotification;
// @ts-expect-error itemTypeResults is non-null.
export const rejectCompletedNullResults = { importId: "id", itemTypeResults: null } satisfies ExternalAgentConfigImportCompletedNotification;
// @ts-expect-error itemTypeResults must be an array.
export const rejectCompletedStringResults = { importId: "id", itemTypeResults: "results" } satisfies ExternalAgentConfigImportCompletedNotification;
// @ts-expect-error nested item types remain closed.
export const rejectCompletedInvalidNested = { importId: "id", itemTypeResults: [{ itemType: "OTHER", successes: [], failures: [] }] } satisfies ExternalAgentConfigImportCompletedNotification;
// @ts-expect-error snake-case aliases do not replace canonical fields.
export const rejectCompletedSnakeAlias = { import_id: "id", item_type_results: [] } satisfies ExternalAgentConfigImportCompletedNotification;
// @ts-expect-error fields absent from the public record are rejected.
export const rejectCompletedExtra = { ...completedOrderedDuplicates, extra: true } satisfies ExternalAgentConfigImportCompletedNotification;

void (null as unknown as Contracts);
