import type {
  ConfigLayer,
  ConfigLayerMetadata,
  ConfigLayerSource,
  JsonValue,
  OverriddenMetadata,
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

type ConfigLayerMetadataContract = Expect<Equal<ConfigLayerMetadata, {
  name: ConfigLayerSource;
  version: string;
}>>;
type ConfigLayerContract = Expect<Equal<ConfigLayer, {
  name: ConfigLayerSource;
  version: string;
  config: JsonValue;
  disabledReason: string | null;
}>>;
type OverriddenMetadataContract = Expect<Equal<OverriddenMetadata, {
  message: string;
  overridingLayer: ConfigLayerMetadata;
  effectiveValue: JsonValue;
}>>;

({ name: { type: "sessionFlags" }, version: "" }) satisfies ConfigLayerMetadata;
({
  name: { type: "user", file: "/home/user/.codex/config.toml", profile: null },
  version: "v1",
}) satisfies ConfigLayerMetadata;
({
  name: { type: "project", dotCodexFolder: "/workspace/.codex" },
  version: "v2",
  config: { nested: [9007199254740993, true, null] },
  disabledReason: null,
}) satisfies ConfigLayer;
({
  name: { type: "system", file: "/etc/codex/config.toml" },
  version: "v3",
  config: [],
  disabledReason: "policy",
}) satisfies ConfigLayer;
({
  message: "",
  overridingLayer: { name: { type: "sessionFlags" }, version: "v1" },
  effectiveValue: null,
}) satisfies OverriddenMetadata;
({
  message: "overridden",
  overridingLayer: { name: { type: "legacyManagedConfigTomlFromMdm" }, version: "v2" },
  effectiveValue: { nested: ["value", false] },
}) satisfies OverriddenMetadata;

// @ts-expect-error metadata requires name.
({ version: "v1" }) satisfies ConfigLayerMetadata;
// @ts-expect-error metadata name is non-null.
({ name: null, version: "v1" }) satisfies ConfigLayerMetadata;
// @ts-expect-error metadata requires version.
({ name: { type: "sessionFlags" } }) satisfies ConfigLayerMetadata;
// @ts-expect-error metadata version is a string.
({ name: { type: "sessionFlags" }, version: 1 }) satisfies ConfigLayerMetadata;
// @ts-expect-error metadata is closed.
({ name: { type: "sessionFlags" }, version: "v1", extra: true }) satisfies ConfigLayerMetadata;

// @ts-expect-error layer requires name.
({ version: "v1", config: null, disabledReason: null }) satisfies ConfigLayer;
// @ts-expect-error layer source remains exact.
({ name: { type: "sessionFlags", extra: true }, version: "v1", config: null, disabledReason: null }) satisfies ConfigLayer;
// @ts-expect-error layer requires version.
({ name: { type: "sessionFlags" }, config: null, disabledReason: null }) satisfies ConfigLayer;
// @ts-expect-error layer requires config.
({ name: { type: "sessionFlags" }, version: "v1", disabledReason: null }) satisfies ConfigLayer;
// @ts-expect-error config cannot be undefined.
({ name: { type: "sessionFlags" }, version: "v1", config: undefined, disabledReason: null }) satisfies ConfigLayer;
// @ts-expect-error disabledReason is required nullable.
({ name: { type: "sessionFlags" }, version: "v1", config: null }) satisfies ConfigLayer;
// @ts-expect-error disabledReason is a nullable string.
({ name: { type: "sessionFlags" }, version: "v1", config: null, disabledReason: 1 }) satisfies ConfigLayer;
// @ts-expect-error layer is closed.
({ name: { type: "sessionFlags" }, version: "v1", config: null, disabledReason: null, extra: true }) satisfies ConfigLayer;

// @ts-expect-error overridden metadata requires message.
({ overridingLayer: { name: { type: "sessionFlags" }, version: "v1" }, effectiveValue: null }) satisfies OverriddenMetadata;
// @ts-expect-error overridden metadata requires overridingLayer.
({ message: "value", effectiveValue: null }) satisfies OverriddenMetadata;
// @ts-expect-error overridingLayer remains exact.
({ message: "value", overridingLayer: { name: { type: "sessionFlags" } }, effectiveValue: null }) satisfies OverriddenMetadata;
// @ts-expect-error overridden metadata requires effectiveValue.
({ message: "value", overridingLayer: { name: { type: "sessionFlags" }, version: "v1" } }) satisfies OverriddenMetadata;
// @ts-expect-error overridden metadata is closed.
({ message: "value", overridingLayer: { name: { type: "sessionFlags" }, version: "v1" }, effectiveValue: null, extra: true }) satisfies OverriddenMetadata;

declare const metadataContract: ConfigLayerMetadataContract;
declare const layerContract: ConfigLayerContract;
declare const overriddenContract: OverriddenMetadataContract;
void metadataContract;
void layerContract;
void overriddenContract;
