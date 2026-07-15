import type {
  ExternalAgentConfigDetectParams,
  ExternalAgentConfigDetectResponse,
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
  Expect<Equal<ExternalAgentConfigDetectParams, {
    cwds?: Array<string> | null;
    includeHome?: boolean;
  }>>,
  Expect<Equal<ExternalAgentConfigDetectResponse, {
    items: Array<ExternalAgentConfigMigrationItem>;
  }>>,
  Expect<Equal<"externalAgentConfig/detect" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/detect" extends keyof MethodResultsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/import" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/import" extends keyof MethodResultsByName ? true : false, false>>,
];

export const defaults = {} satisfies ExternalAgentConfigDetectParams;
export const home = { includeHome: true, cwds: null } satisfies ExternalAgentConfigDetectParams;
export const repos = { cwds: ["", "repo/../repo", "/tmp/repo", ""] } satisfies ExternalAgentConfigDetectParams;

export const item = {
  itemType: "CONFIG",
  description: "config",
  cwd: null,
  details: null,
} satisfies ExternalAgentConfigMigrationItem;
export const response = { items: [item, item] } satisfies ExternalAgentConfigDetectResponse;

// @ts-expect-error includeHome is a non-null boolean.
export const rejectNullIncludeHome = { includeHome: null } satisfies ExternalAgentConfigDetectParams;
// @ts-expect-error includeHome is a boolean.
export const rejectNumericIncludeHome = { includeHome: 1 } satisfies ExternalAgentConfigDetectParams;
// @ts-expect-error optional fields do not accept explicit undefined.
export const rejectUndefinedIncludeHome = { includeHome: undefined } satisfies ExternalAgentConfigDetectParams;
// @ts-expect-error cwds is a nullable string array.
export const rejectScalarCwds = { cwds: "repo" } satisfies ExternalAgentConfigDetectParams;
// @ts-expect-error cwd entries are non-null strings.
export const rejectNullCwd = { cwds: [null] } satisfies ExternalAgentConfigDetectParams;
// @ts-expect-error cwd entries are strings.
export const rejectNumericCwd = { cwds: [1] } satisfies ExternalAgentConfigDetectParams;
// @ts-expect-error optional fields do not accept explicit undefined.
export const rejectUndefinedCwds = { cwds: undefined } satisfies ExternalAgentConfigDetectParams;
// @ts-expect-error snake-case aliases do not replace canonical fields.
export const rejectSnakeAlias = { include_home: true } satisfies ExternalAgentConfigDetectParams;
// @ts-expect-error fields absent from the public request are rejected.
export const rejectParamsExtra = { includeHome: true, extra: true } satisfies ExternalAgentConfigDetectParams;

// @ts-expect-error items is required.
export const rejectMissingItems = {} satisfies ExternalAgentConfigDetectResponse;
// @ts-expect-error items is non-null.
export const rejectNullItems = { items: null } satisfies ExternalAgentConfigDetectResponse;
// @ts-expect-error items is an array.
export const rejectScalarItems = { items: item } satisfies ExternalAgentConfigDetectResponse;
// @ts-expect-error response items retain strict migration-item fields.
export const rejectMalformedItem = { items: [{ itemType: "CONFIG", description: "config" }] } satisfies ExternalAgentConfigDetectResponse;
// @ts-expect-error fields absent from the public response are rejected.
export const rejectResponseExtra = { items: [], extra: true } satisfies ExternalAgentConfigDetectResponse;

void (null as unknown as Contracts);
