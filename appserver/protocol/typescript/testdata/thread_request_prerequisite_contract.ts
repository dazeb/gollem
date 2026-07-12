import type {
  Personality,
  SandboxMode,
  ThreadStartSource,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type Contracts = [
  Expect<Equal<Personality, "none" | "friendly" | "pragmatic">>,
  Expect<Equal<SandboxMode, "read-only" | "workspace-write" | "danger-full-access">>,
  Expect<Equal<ThreadStartSource, "startup" | "clear">>,
];

export const personalities: Personality[] = ["none", "friendly", "pragmatic"];
export const sandboxModes: SandboxMode[] = ["read-only", "workspace-write", "danger-full-access"];
export const startSources: ThreadStartSource[] = ["startup", "clear"];

// @ts-expect-error personalities are a closed string union.
export const rejectUnknownPersonality = "concise" satisfies Personality;
// @ts-expect-error sandbox modes use exact kebab-case spelling.
export const rejectCamelSandbox = "workspaceWrite" satisfies SandboxMode;
// @ts-expect-error thread start sources are a closed string union.
export const rejectUnknownStartSource = "resume" satisfies ThreadStartSource;
// @ts-expect-error sandbox policy discriminants are not sandbox modes.
export const rejectPolicyDiscriminant = "readOnly" satisfies SandboxMode;
// @ts-expect-error the three enums remain distinct.
export const rejectPersonalityAsSandbox = "none" satisfies SandboxMode;
// @ts-expect-error the three enums remain distinct.
export const rejectSandboxAsStartSource = "read-only" satisfies ThreadStartSource;
// @ts-expect-error the three enums remain distinct.
export const rejectStartSourceAsPersonality = "startup" satisfies Personality;
// @ts-expect-error generated enum values are non-null.
export const rejectNullPersonality = null satisfies Personality;

void (null as unknown as Contracts);
