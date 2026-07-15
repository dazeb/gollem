import type {
  ExternalAgentConfigMigrationItem,
  ExternalAgentConfigMigrationItemType,
  MethodParamsByName,
  MethodResultsByName,
  MigrationDetails,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type Expected = {
  itemType: ExternalAgentConfigMigrationItemType;
  description: string;
  cwd: string | null;
  details: MigrationDetails | null;
};
type Contracts = [
  Expect<Equal<ExternalAgentConfigMigrationItem, Expected>>,
  Expect<Equal<"externalAgentConfig/detect" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/detect" extends keyof MethodResultsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/import" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/import" extends keyof MethodResultsByName ? true : false, false>>,
];

const emptyDetails = {
  plugins: [],
  skills: [],
  sessions: [],
  mcpServers: [],
  hooks: [],
  subagents: [],
  commands: [],
} satisfies MigrationDetails;

export const home = {
  itemType: "AGENTS_MD",
  description: "agents",
  cwd: null,
  details: null,
} satisfies ExternalAgentConfigMigrationItem;

export const repo = {
  itemType: "SKILLS",
  description: "skills",
  cwd: "repo/../repo",
  details: { ...emptyDetails, skills: [{ name: "one" }] },
} satisfies ExternalAgentConfigMigrationItem;

// @ts-expect-error itemType is required.
export const rejectMissingItemType = { description: "config", cwd: null, details: null } satisfies ExternalAgentConfigMigrationItem;
// @ts-expect-error description is required.
export const rejectMissingDescription = { itemType: "CONFIG", cwd: null, details: null } satisfies ExternalAgentConfigMigrationItem;
// @ts-expect-error cwd remains required-nullable in generated TypeScript.
export const rejectMissingCwd = { itemType: "CONFIG", description: "config", details: null } satisfies ExternalAgentConfigMigrationItem;
// @ts-expect-error details remains required-nullable in generated TypeScript.
export const rejectMissingDetails = { itemType: "CONFIG", description: "config", cwd: null } satisfies ExternalAgentConfigMigrationItem;
// @ts-expect-error itemType is non-null.
export const rejectNullItemType = { ...home, itemType: null } satisfies ExternalAgentConfigMigrationItem;
// @ts-expect-error itemType is closed.
export const rejectUnknownItemType = { ...home, itemType: "OTHER" } satisfies ExternalAgentConfigMigrationItem;
// @ts-expect-error description is non-null.
export const rejectNullDescription = { ...home, description: null } satisfies ExternalAgentConfigMigrationItem;
// @ts-expect-error cwd is nullable string data.
export const rejectNumericCwd = { ...home, cwd: 1 } satisfies ExternalAgentConfigMigrationItem;
// @ts-expect-error details is nullable MigrationDetails.
export const rejectArrayDetails = { ...home, details: [] } satisfies ExternalAgentConfigMigrationItem;
// @ts-expect-error nested details remain strict.
export const rejectMalformedDetails = { ...home, details: { ...emptyDetails, skills: [{}] } } satisfies ExternalAgentConfigMigrationItem;
// @ts-expect-error snake-case aliases do not replace canonical fields.
export const rejectSnakeAlias = { item_type: "CONFIG", description: "config", cwd: null, details: null } satisfies ExternalAgentConfigMigrationItem;
// @ts-expect-error fields absent from the public record are rejected.
export const rejectExtra = { ...home, extra: true } satisfies ExternalAgentConfigMigrationItem;

void (null as unknown as Contracts);
