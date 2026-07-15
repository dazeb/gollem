import type {
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

type Expected =
  | "AGENTS_MD"
  | "CONFIG"
  | "SKILLS"
  | "PLUGINS"
  | "MCP_SERVER_CONFIG"
  | "SUBAGENTS"
  | "HOOKS"
  | "COMMANDS"
  | "SESSIONS";
type Contracts = [
  Expect<Equal<ExternalAgentConfigMigrationItemType, Expected>>,
  Expect<Equal<"externalAgentConfig/detect" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/detect" extends keyof MethodResultsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/import" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/import" extends keyof MethodResultsByName ? true : false, false>>,
];

export const values = [
  "AGENTS_MD",
  "CONFIG",
  "SKILLS",
  "PLUGINS",
  "MCP_SERVER_CONFIG",
  "SUBAGENTS",
  "HOOKS",
  "COMMANDS",
  "SESSIONS",
] satisfies ExternalAgentConfigMigrationItemType[];

// @ts-expect-error item types are closed.
export const rejectUnknown = "OTHER" satisfies ExternalAgentConfigMigrationItemType;
// @ts-expect-error empty strings are not item types.
export const rejectEmpty = "" satisfies ExternalAgentConfigMigrationItemType;
// @ts-expect-error exact uppercase spelling is required.
export const rejectLowercase = "config" satisfies ExternalAgentConfigMigrationItemType;
// @ts-expect-error camel-case aliases are rejected.
export const rejectCamelCase = "agentsMd" satisfies ExternalAgentConfigMigrationItemType;
// @ts-expect-error singular aliases are rejected.
export const rejectSingular = "SESSION" satisfies ExternalAgentConfigMigrationItemType;
// @ts-expect-error exact MCP spelling is required.
export const rejectMcpAlias = "MCP_SERVER" satisfies ExternalAgentConfigMigrationItemType;
// @ts-expect-error item types are non-null.
export const rejectNull = null satisfies ExternalAgentConfigMigrationItemType;
// @ts-expect-error item types are strings.
export const rejectNumber = 1 satisfies ExternalAgentConfigMigrationItemType;
// @ts-expect-error item types are strings.
export const rejectBoolean = true satisfies ExternalAgentConfigMigrationItemType;
// @ts-expect-error item types are strings.
export const rejectObject = {} satisfies ExternalAgentConfigMigrationItemType;
// @ts-expect-error item types are strings.
export const rejectArray = [] satisfies ExternalAgentConfigMigrationItemType;

void (null as unknown as Contracts);
