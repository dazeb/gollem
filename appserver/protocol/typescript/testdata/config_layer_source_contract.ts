import type {
  AbsolutePathBuf,
  ConfigLayerSource,
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

type ConfigLayerSourceContract = Expect<Equal<ConfigLayerSource,
  | { type: "mdm"; domain: string; key: string }
  | { type: "system"; file: AbsolutePathBuf }
  | { type: "enterpriseManaged"; id: string; name: string }
  | { type: "user"; file: AbsolutePathBuf; profile: string | null }
  | { type: "project"; dotCodexFolder: AbsolutePathBuf }
  | { type: "sessionFlags" }
  | { type: "legacyManagedConfigTomlFromFile"; file: AbsolutePathBuf }
  | { type: "legacyManagedConfigTomlFromMdm" }
>>;

({ type: "mdm", domain: "", key: "" }) satisfies ConfigLayerSource;
({ type: "system", file: "/etc/codex/config.toml" }) satisfies ConfigLayerSource;
({ type: "enterpriseManaged", id: "", name: "Managed" }) satisfies ConfigLayerSource;
({ type: "user", file: "/home/user/.codex/config.toml", profile: null }) satisfies ConfigLayerSource;
({ type: "user", file: "/home/user/.codex/config.toml", profile: "" }) satisfies ConfigLayerSource;
({ type: "project", dotCodexFolder: "/workspace/.codex" }) satisfies ConfigLayerSource;
({ type: "sessionFlags" }) satisfies ConfigLayerSource;
({ type: "legacyManagedConfigTomlFromFile", file: "/etc/codex/managed_config.toml" }) satisfies ConfigLayerSource;
({ type: "legacyManagedConfigTomlFromMdm" }) satisfies ConfigLayerSource;

// @ts-expect-error discriminants are closed.
({ type: "unknown" }) satisfies ConfigLayerSource;
// @ts-expect-error the discriminant is required.
({ domain: "d", key: "k" }) satisfies ConfigLayerSource;
// @ts-expect-error mdm requires domain.
({ type: "mdm", key: "k" }) satisfies ConfigLayerSource;
// @ts-expect-error mdm requires a string key.
({ type: "mdm", domain: "d", key: 1 }) satisfies ConfigLayerSource;
// @ts-expect-error variants are closed.
({ type: "mdm", domain: "d", key: "k", extra: true }) satisfies ConfigLayerSource;
// @ts-expect-error system requires file.
({ type: "system" }) satisfies ConfigLayerSource;
// @ts-expect-error system file is non-null.
({ type: "system", file: null }) satisfies ConfigLayerSource;
// @ts-expect-error enterpriseManaged requires name.
({ type: "enterpriseManaged", id: "id" }) satisfies ConfigLayerSource;
// @ts-expect-error enterpriseManaged name is non-null.
({ type: "enterpriseManaged", id: "id", name: null }) satisfies ConfigLayerSource;
// @ts-expect-error user profile is required nullable.
({ type: "user", file: "/config.toml" }) satisfies ConfigLayerSource;
// @ts-expect-error user profile is a nullable string.
({ type: "user", file: "/config.toml", profile: 1 }) satisfies ConfigLayerSource;
// @ts-expect-error project requires dotCodexFolder.
({ type: "project" }) satisfies ConfigLayerSource;
// @ts-expect-error type-only variants are closed.
({ type: "sessionFlags", file: "/config.toml" }) satisfies ConfigLayerSource;
// @ts-expect-error legacy file source requires file.
({ type: "legacyManagedConfigTomlFromFile" }) satisfies ConfigLayerSource;
// @ts-expect-error legacy MDM source is type-only.
({ type: "legacyManagedConfigTomlFromMdm", file: "/config.toml" }) satisfies ConfigLayerSource;
// @ts-expect-error fields cannot cross variants.
({ type: "project", file: "/workspace/.codex" }) satisfies ConfigLayerSource;

declare const contract: ConfigLayerSourceContract;
void contract;
