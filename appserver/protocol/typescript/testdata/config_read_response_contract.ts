import type {
  Config,
  ConfigLayer,
  ConfigLayerMetadata,
  ConfigReadResponse,
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

type ConfigReadResponseShape = {
  config: Config;
  origins: { [key in string]?: ConfigLayerMetadata };
  layers: ConfigLayer[] | null;
};

type ConfigReadResponseContract = Expect<
  Equal<ConfigReadResponse, ConfigReadResponseShape>
>;

declare const config: Config;

({ config, origins: {}, layers: null }) satisfies ConfigReadResponse;
({
  config,
  origins: {
    "": { name: { type: "sessionFlags" }, version: "" },
    model: {
      name: { type: "user", file: "/home/user/.codex/config.toml", profile: null },
      version: "v1",
    },
  },
  layers: [
    {
      name: { type: "project", dotCodexFolder: "/workspace/.codex" },
      version: "v2",
      config: { integer: 9007199254740993 },
      disabledReason: null,
    },
    {
      name: { type: "sessionFlags" },
      version: "v3",
      config: null,
      disabledReason: "policy",
    },
  ],
}) satisfies ConfigReadResponse;

// @ts-expect-error config is required.
({ origins: {}, layers: null }) satisfies ConfigReadResponse;
// @ts-expect-error config is non-null.
({ config: null, origins: {}, layers: null }) satisfies ConfigReadResponse;
// @ts-expect-error origins is required.
({ config, layers: null }) satisfies ConfigReadResponse;
// @ts-expect-error origins is non-null.
({ config, origins: null, layers: null }) satisfies ConfigReadResponse;
// @ts-expect-error origins is an object map.
({ config, origins: [], layers: null }) satisfies ConfigReadResponse;
// @ts-expect-error origin values are non-null metadata.
({ config, origins: { model: null }, layers: null }) satisfies ConfigReadResponse;
// @ts-expect-error origin metadata requires version.
({ config, origins: { model: { name: { type: "sessionFlags" } } }, layers: null }) satisfies ConfigReadResponse;
// @ts-expect-error origin metadata is closed.
({ config, origins: { model: { name: { type: "sessionFlags" }, version: "v1", extra: true } }, layers: null }) satisfies ConfigReadResponse;
// @ts-expect-error layers is generated as required nullable.
({ config, origins: {} }) satisfies ConfigReadResponse;
// @ts-expect-error layers cannot be explicit undefined.
({ config, origins: {}, layers: undefined }) satisfies ConfigReadResponse;
// @ts-expect-error layers is an array or null.
({ config, origins: {}, layers: {} }) satisfies ConfigReadResponse;
// @ts-expect-error layer entries are non-null.
({ config, origins: {}, layers: [null] }) satisfies ConfigReadResponse;
// @ts-expect-error layer entries require name.
({ config, origins: {}, layers: [{ version: "v1", config: null, disabledReason: null }] }) satisfies ConfigReadResponse;
// @ts-expect-error layer entries are closed.
({ config, origins: {}, layers: [{ name: { type: "sessionFlags" }, version: "v1", config: null, disabledReason: null, extra: true }] }) satisfies ConfigReadResponse;
// @ts-expect-error config known fields retain their exact types.
({ config: { ...config, model: 1 }, origins: {}, layers: null }) satisfies ConfigReadResponse;
// @ts-expect-error config additional fields must be JSON.
({ config: { ...config, future: () => true }, origins: {}, layers: null }) satisfies ConfigReadResponse;
// @ts-expect-error origin values are metadata, not callbacks.
({ config, origins: { model: () => true }, layers: null }) satisfies ConfigReadResponse;
// @ts-expect-error nested layer config must be JSON.
({ config, origins: {}, layers: [{ name: { type: "sessionFlags" }, version: "v1", config: () => true, disabledReason: null }] }) satisfies ConfigReadResponse;
// @ts-expect-error generated response TypeScript remains closed.
({ config, origins: {}, layers: null, future: true }) satisfies ConfigReadResponse;
// @ts-expect-error ConfigReadResponse is an object.
null satisfies ConfigReadResponse;

declare const configReadResponseContract: ConfigReadResponseContract;
void configReadResponseContract;
