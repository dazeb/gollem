import type {
  ConfigBatchWriteParams,
  ConfigEdit,
  ConfigValueWriteParams,
  ConfigWriteResponse,
  JsonValue,
  MergeStrategy,
  OverriddenMetadata,
  WriteStatus,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? (<T>() => T extends B ? 1 : 2) extends
        (<T>() => T extends A ? 1 : 2)
      ? true
      : false
    : false;
type Expect<T extends true> = T;

type MergeStrategyContract = Expect<Equal<MergeStrategy, "replace" | "upsert">>;
type WriteStatusContract = Expect<Equal<WriteStatus, "ok" | "okOverridden">>;
type ConfigEditContract = Expect<Equal<ConfigEdit, {
  keyPath: string;
  value: JsonValue;
  mergeStrategy: MergeStrategy;
}>>;
type ConfigValueWriteParamsContract = Expect<Equal<ConfigValueWriteParams, {
  keyPath: string;
  value: JsonValue;
  mergeStrategy: MergeStrategy;
  filePath?: string | null;
  expectedVersion?: string | null;
}>>;
type ConfigBatchWriteParamsContract = Expect<Equal<ConfigBatchWriteParams, {
  edits: ConfigEdit[];
  filePath?: string | null;
  expectedVersion?: string | null;
  reloadUserConfig?: boolean;
}>>;
type ConfigWriteResponseContract = Expect<Equal<ConfigWriteResponse, {
  status: WriteStatus;
  version: string;
  filePath: string;
  overriddenMetadata: OverriddenMetadata | null;
}>>;

"replace" satisfies MergeStrategy;
"upsert" satisfies MergeStrategy;
"ok" satisfies WriteStatus;
"okOverridden" satisfies WriteStatus;
({ keyPath: "", value: null, mergeStrategy: "replace" }) satisfies ConfigEdit;
({
  keyPath: "model.reasoning",
  value: { nested: [9007199254740993, true, null] },
  mergeStrategy: "upsert",
}) satisfies ConfigEdit;
({ keyPath: "model", value: null, mergeStrategy: "replace" }) satisfies ConfigValueWriteParams;
({
  keyPath: "model",
  value: [],
  mergeStrategy: "upsert",
  filePath: null,
  expectedVersion: "",
}) satisfies ConfigValueWriteParams;
({ edits: [] }) satisfies ConfigBatchWriteParams;
({
  edits: [
    { keyPath: "model", value: "first", mergeStrategy: "replace" },
    { keyPath: "model", value: "second", mergeStrategy: "upsert" },
  ],
  filePath: null,
  expectedVersion: "v1",
  reloadUserConfig: false,
}) satisfies ConfigBatchWriteParams;
({
  status: "ok",
  version: "",
  filePath: "/workspace/config.toml",
  overriddenMetadata: null,
}) satisfies ConfigWriteResponse;
({
  status: "okOverridden",
  version: "v2",
  filePath: "/workspace/config.toml",
  overriddenMetadata: {
    message: "overridden",
    overridingLayer: { name: { type: "sessionFlags" }, version: "managed" },
    effectiveValue: { nested: [9007199254740993, false] },
  },
}) satisfies ConfigWriteResponse;

// @ts-expect-error merge strategy is closed.
"merge" satisfies MergeStrategy;
// @ts-expect-error write status is closed.
"accepted" satisfies WriteStatus;

// @ts-expect-error edit requires keyPath.
({ value: null, mergeStrategy: "replace" }) satisfies ConfigEdit;
// @ts-expect-error edit requires value.
({ keyPath: "model", mergeStrategy: "replace" }) satisfies ConfigEdit;
// @ts-expect-error edit value is JSON, not undefined.
({ keyPath: "model", value: undefined, mergeStrategy: "replace" }) satisfies ConfigEdit;
// @ts-expect-error edit merge strategy is exact.
({ keyPath: "model", value: null, mergeStrategy: "merge" }) satisfies ConfigEdit;
// @ts-expect-error edit is closed.
({ keyPath: "model", value: null, mergeStrategy: "replace", extra: true }) satisfies ConfigEdit;

// @ts-expect-error value write requires keyPath.
({ value: null, mergeStrategy: "replace" }) satisfies ConfigValueWriteParams;
// @ts-expect-error value write requires value.
({ keyPath: "model", mergeStrategy: "replace" }) satisfies ConfigValueWriteParams;
// @ts-expect-error value write requires mergeStrategy.
({ keyPath: "model", value: null }) satisfies ConfigValueWriteParams;
// @ts-expect-error keyPath is a string.
({ keyPath: 1, value: null, mergeStrategy: "replace" }) satisfies ConfigValueWriteParams;
// @ts-expect-error optional filePath cannot be explicit undefined.
({ keyPath: "model", value: null, mergeStrategy: "replace", filePath: undefined }) satisfies ConfigValueWriteParams;
// @ts-expect-error expectedVersion is a nullable string.
({ keyPath: "model", value: null, mergeStrategy: "replace", expectedVersion: false }) satisfies ConfigValueWriteParams;
// @ts-expect-error value write is closed.
({ keyPath: "model", value: null, mergeStrategy: "replace", key: "model" }) satisfies ConfigValueWriteParams;

// @ts-expect-error batch write requires edits.
({}) satisfies ConfigBatchWriteParams;
// @ts-expect-error edits are non-null.
({ edits: null }) satisfies ConfigBatchWriteParams;
// @ts-expect-error edit members are non-null.
({ edits: [null] }) satisfies ConfigBatchWriteParams;
// @ts-expect-error nested edits remain exact.
({ edits: [{ keyPath: "model", value: null, mergeStrategy: "merge" }] }) satisfies ConfigBatchWriteParams;
// @ts-expect-error optional filePath cannot be explicit undefined.
({ edits: [], filePath: undefined }) satisfies ConfigBatchWriteParams;
// @ts-expect-error reloadUserConfig is a non-null boolean.
({ edits: [], reloadUserConfig: null }) satisfies ConfigBatchWriteParams;
// @ts-expect-error batch write is closed.
({ edits: [], entries: [] }) satisfies ConfigBatchWriteParams;

// @ts-expect-error response requires status.
({ version: "v1", filePath: "/config.toml", overriddenMetadata: null }) satisfies ConfigWriteResponse;
// @ts-expect-error response status is exact.
({ status: "accepted", version: "v1", filePath: "/config.toml", overriddenMetadata: null }) satisfies ConfigWriteResponse;
// @ts-expect-error response requires version.
({ status: "ok", filePath: "/config.toml", overriddenMetadata: null }) satisfies ConfigWriteResponse;
// @ts-expect-error response requires filePath.
({ status: "ok", version: "v1", overriddenMetadata: null }) satisfies ConfigWriteResponse;
// @ts-expect-error filePath is a string.
({ status: "ok", version: "v1", filePath: 1, overriddenMetadata: null }) satisfies ConfigWriteResponse;
// @ts-expect-error overriddenMetadata is required nullable.
({ status: "ok", version: "v1", filePath: "/config.toml" }) satisfies ConfigWriteResponse;
// @ts-expect-error nested override metadata remains exact.
({ status: "ok", version: "v1", filePath: "/config.toml", overriddenMetadata: { message: "value", overridingLayer: { name: { type: "sessionFlags" }, version: "v1" } } }) satisfies ConfigWriteResponse;
// @ts-expect-error response is closed.
({ status: "ok", version: "v1", filePath: "/config.toml", overriddenMetadata: null, extra: true }) satisfies ConfigWriteResponse;

declare const mergeStrategyContract: MergeStrategyContract;
declare const writeStatusContract: WriteStatusContract;
declare const configEditContract: ConfigEditContract;
declare const valueWriteContract: ConfigValueWriteParamsContract;
declare const batchWriteContract: ConfigBatchWriteParamsContract;
declare const responseContract: ConfigWriteResponseContract;
void mergeStrategyContract;
void writeStatusContract;
void configEditContract;
void valueWriteContract;
void batchWriteContract;
void responseContract;
