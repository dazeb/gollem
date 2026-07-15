import type {
  MethodParamsByName,
  MethodResultsByName,
  PluginsMigration,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type Expected = {
  marketplaceName: string;
  pluginNames: Array<string>;
};
type Contracts = [
  Expect<Equal<PluginsMigration, Expected>>,
  Expect<Equal<"externalAgentConfig/detect" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/detect" extends keyof MethodResultsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/import" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"externalAgentConfig/import" extends keyof MethodResultsByName ? true : false, false>>,
];

export const empty = { marketplaceName: "", pluginNames: [] } satisfies PluginsMigration;
export const named = {
  marketplaceName: "market",
  pluginNames: ["one", "", "one"],
} satisfies PluginsMigration;

// @ts-expect-error both fields are required.
export const rejectMissing = {} satisfies PluginsMigration;
// @ts-expect-error marketplaceName is required.
export const rejectMissingMarketplace = { pluginNames: [] } satisfies PluginsMigration;
// @ts-expect-error pluginNames is required.
export const rejectMissingPlugins = { marketplaceName: "market" } satisfies PluginsMigration;
// @ts-expect-error marketplaceName is non-null.
export const rejectNullMarketplace = { marketplaceName: null, pluginNames: [] } satisfies PluginsMigration;
// @ts-expect-error marketplaceName must be a string.
export const rejectNumberMarketplace = { marketplaceName: 1, pluginNames: [] } satisfies PluginsMigration;
// @ts-expect-error pluginNames is non-null.
export const rejectNullPlugins = { marketplaceName: "market", pluginNames: null } satisfies PluginsMigration;
// @ts-expect-error pluginNames must be an array.
export const rejectStringPlugins = { marketplaceName: "market", pluginNames: "one" } satisfies PluginsMigration;
// @ts-expect-error plugin names must be strings.
export const rejectNumberPlugin = { marketplaceName: "market", pluginNames: [1] } satisfies PluginsMigration;
// @ts-expect-error snake-case aliases do not replace canonical fields.
export const rejectAliases = { marketplace_name: "market", plugin_names: [] } satisfies PluginsMigration;
// @ts-expect-error fields absent from the public record are rejected.
export const rejectExtra = { marketplaceName: "market", pluginNames: [], extra: true } satisfies PluginsMigration;

void (null as unknown as Contracts);
