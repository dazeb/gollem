import type {
  CommandMigration,
  HookMigration,
  McpServerMigration,
  MethodParamsByName,
  MethodResultsByName,
  MigrationDetails,
  PluginsMigration,
  SessionMigration,
  SkillMigration,
  SubagentMigration,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type Expected = {
  plugins: Array<PluginsMigration>;
  skills: Array<SkillMigration>;
  sessions: Array<SessionMigration>;
  mcpServers: Array<McpServerMigration>;
  hooks: Array<HookMigration>;
  subagents: Array<SubagentMigration>;
  commands: Array<CommandMigration>;
};
type Contracts = [
  Expect<Equal<MigrationDetails, Expected>>,
  Expect<Equal<"externalAgentConfig/detect" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/detect" extends keyof MethodResultsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/import" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/import" extends keyof MethodResultsByName ? true : false, false>>,
];

export const empty = {
  plugins: [],
  skills: [],
  sessions: [],
  mcpServers: [],
  hooks: [],
  subagents: [],
  commands: [],
} satisfies MigrationDetails;

export const full = {
  plugins: [{ marketplaceName: "market", pluginNames: ["one", "one"] }],
  skills: [{ name: "skill" }],
  sessions: [{ path: "session", cwd: "repo", title: null }],
  mcpServers: [{ name: "mcp" }],
  hooks: [{ name: "hook" }],
  subagents: [{ name: "agent" }],
  commands: [{ name: "command" }, { name: "command" }],
} satisfies MigrationDetails;

// @ts-expect-error all seven fields are required in generated TypeScript.
export const rejectOmitted = {} satisfies MigrationDetails;
// @ts-expect-error partial records are rejected in generated TypeScript.
export const rejectPartial = { commands: [] } satisfies MigrationDetails;
// @ts-expect-error plugins is non-null.
export const rejectNullPlugins = { ...empty, plugins: null } satisfies MigrationDetails;
// @ts-expect-error skills is non-null.
export const rejectNullSkills = { ...empty, skills: null } satisfies MigrationDetails;
// @ts-expect-error sessions is non-null.
export const rejectNullSessions = { ...empty, sessions: null } satisfies MigrationDetails;
// @ts-expect-error mcpServers is non-null.
export const rejectNullMcpServers = { ...empty, mcpServers: null } satisfies MigrationDetails;
// @ts-expect-error hooks is non-null.
export const rejectNullHooks = { ...empty, hooks: null } satisfies MigrationDetails;
// @ts-expect-error subagents is non-null.
export const rejectNullSubagents = { ...empty, subagents: null } satisfies MigrationDetails;
// @ts-expect-error commands is non-null.
export const rejectNullCommands = { ...empty, commands: null } satisfies MigrationDetails;
// @ts-expect-error nested command values remain strict.
export const rejectMalformedCommand = { ...empty, commands: [{}] } satisfies MigrationDetails;
// @ts-expect-error snake-case aliases do not replace the canonical field.
export const rejectSnakeAlias = { ...empty, mcpServers: undefined, mcp_servers: [] } satisfies MigrationDetails;
// @ts-expect-error fields absent from the public record are rejected.
export const rejectExtra = { ...empty, extra: true } satisfies MigrationDetails;

void (null as unknown as Contracts);
