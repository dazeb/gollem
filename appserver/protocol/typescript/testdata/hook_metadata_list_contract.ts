import type {
  HookErrorInfo,
  HookMetadata,
  HooksListEntry,
  HooksListParams,
  HooksListResponse,
  MethodParamsByName,
  MethodResultsByName,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type Metadata = {
  key: string;
  eventName: "preToolUse" | "permissionRequest" | "postToolUse" | "preCompact" | "postCompact" | "sessionStart" | "userPromptSubmit" | "subagentStart" | "subagentStop" | "stop";
  handlerType: "command" | "prompt" | "agent";
  matcher: string | null;
  command: string | null;
  timeoutSec: bigint;
  statusMessage: string | null;
  sourcePath: string;
  source: "system" | "user" | "project" | "mdm" | "sessionFlags" | "plugin" | "cloudRequirements" | "cloudManagedConfig" | "legacyManagedConfigFile" | "legacyManagedConfigMdm" | "unknown";
  pluginId: string | null;
  displayOrder: bigint;
  enabled: boolean;
  isManaged: boolean;
  currentHash: string;
  trustStatus: "managed" | "untrusted" | "trusted" | "modified";
};

type Contracts = [
  Expect<Equal<HookErrorInfo, { path: string; message: string }>>,
  Expect<Equal<HookMetadata, Metadata>>,
  Expect<Equal<HooksListEntry, {
    cwd: string;
    hooks: Array<HookMetadata>;
    warnings: Array<string>;
    errors: Array<HookErrorInfo>;
  }>>,
  Expect<Equal<HooksListParams, { cwds?: Array<string> }>>,
  Expect<Equal<HooksListResponse, { data: Array<HooksListEntry> }>>,
  Expect<Equal<Extract<keyof MethodParamsByName, "hooks/list">, never>>,
  Expect<Equal<Extract<keyof MethodResultsByName, "hooks/list">, never>>,
];
declare const contracts: Contracts;
void contracts;

const metadata = {
  key: "",
  eventName: "preToolUse",
  handlerType: "command",
  matcher: null,
  command: null,
  timeoutSec: 18_446_744_073_709_551_615n,
  statusMessage: null,
  sourcePath: "/hooks.json",
  source: "unknown",
  pluginId: null,
  displayOrder: -9_223_372_036_854_775_808n,
  enabled: false,
  isManaged: false,
  currentHash: "",
  trustStatus: "untrusted",
} satisfies HookMetadata;
({ path: "", message: "" }) satisfies HookErrorInfo;
({ cwd: "", hooks: [metadata, metadata], warnings: ["", ""], errors: [] }) satisfies HooksListEntry;
({}) satisfies HooksListParams;
({ cwds: [] }) satisfies HooksListParams;
({ cwds: ["", "repo/../repo", ""] }) satisfies HooksListParams;
({ data: [] }) satisfies HooksListResponse;

// @ts-expect-error path is required.
({ message: "" }) satisfies HookErrorInfo;
// @ts-expect-error message is non-null.
({ path: "", message: null }) satisfies HookErrorInfo;
// @ts-expect-error error records are closed.
({ path: "", message: "", extra: true }) satisfies HookErrorInfo;
// @ts-expect-error key is required.
({ ...metadata, key: undefined }) satisfies HookMetadata;
// @ts-expect-error eventName is closed.
({ ...metadata, eventName: "other" }) satisfies HookMetadata;
// @ts-expect-error handlerType is closed.
({ ...metadata, handlerType: "shell" }) satisfies HookMetadata;
// @ts-expect-error matcher is required by exact ts-rs output.
({ ...metadata, matcher: undefined }) satisfies HookMetadata;
// @ts-expect-error matcher is nullable string only.
({ ...metadata, matcher: 1 }) satisfies HookMetadata;
// @ts-expect-error command is required by exact ts-rs output.
({ ...metadata, command: undefined }) satisfies HookMetadata;
// @ts-expect-error timeoutSec is bigint.
({ ...metadata, timeoutSec: 0 }) satisfies HookMetadata;
// @ts-expect-error statusMessage is required by exact ts-rs output.
({ ...metadata, statusMessage: undefined }) satisfies HookMetadata;
// @ts-expect-error sourcePath is required.
({ ...metadata, sourcePath: undefined }) satisfies HookMetadata;
// @ts-expect-error source is required.
({ ...metadata, source: undefined }) satisfies HookMetadata;
// @ts-expect-error source is non-null.
({ ...metadata, source: null }) satisfies HookMetadata;
// @ts-expect-error pluginId is required by exact ts-rs output.
({ ...metadata, pluginId: undefined }) satisfies HookMetadata;
// @ts-expect-error displayOrder is bigint.
({ ...metadata, displayOrder: 0 }) satisfies HookMetadata;
// @ts-expect-error enabled is boolean.
({ ...metadata, enabled: 0 }) satisfies HookMetadata;
// @ts-expect-error isManaged is required.
({ ...metadata, isManaged: undefined }) satisfies HookMetadata;
// @ts-expect-error currentHash is required.
({ ...metadata, currentHash: undefined }) satisfies HookMetadata;
// @ts-expect-error trustStatus is closed.
({ ...metadata, trustStatus: "other" }) satisfies HookMetadata;
// @ts-expect-error snake-case aliases do not replace canonical fields.
({ ...metadata, eventName: undefined, event_name: "preToolUse" }) satisfies HookMetadata;
// @ts-expect-error metadata records are closed.
({ ...metadata, extra: true }) satisfies HookMetadata;
// @ts-expect-error cwd is required.
({ hooks: [], warnings: [], errors: [] }) satisfies HooksListEntry;
// @ts-expect-error hooks are required and non-null.
({ cwd: "", hooks: null, warnings: [], errors: [] }) satisfies HooksListEntry;
// @ts-expect-error nested metadata remains strict.
({ cwd: "", hooks: [{ ...metadata, source: null }], warnings: [], errors: [] }) satisfies HooksListEntry;
// @ts-expect-error warnings are string arrays.
({ cwd: "", hooks: [], warnings: [1], errors: [] }) satisfies HooksListEntry;
// @ts-expect-error nested errors remain strict.
({ cwd: "", hooks: [], warnings: [], errors: [{ path: "" }] }) satisfies HooksListEntry;
// @ts-expect-error entries are closed.
({ cwd: "", hooks: [], warnings: [], errors: [], extra: true }) satisfies HooksListEntry;
// @ts-expect-error cwds is non-null when present.
({ cwds: null }) satisfies HooksListParams;
// @ts-expect-error cwd elements are strings.
({ cwds: [1] }) satisfies HooksListParams;
// @ts-expect-error params are closed.
({ extra: true }) satisfies HooksListParams;
// @ts-expect-error data is required.
({}) satisfies HooksListResponse;
// @ts-expect-error data is non-null.
({ data: null }) satisfies HooksListResponse;
// @ts-expect-error response entries remain strict.
({ data: [{ cwd: "", hooks: [], warnings: [], errors: null }] }) satisfies HooksListResponse;
// @ts-expect-error responses are closed.
({ data: [], extra: true }) satisfies HooksListResponse;
