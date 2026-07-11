import type {
  CommandExecutionItem,
  CommandExecutionSource,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type PublicSourceIsExact = Expect<
  Equal<
    CommandExecutionSource,
    "agent" | "userShell" | "unifiedExecStartup" | "unifiedExecInteraction"
  >
>;
type LiveSourceRemainsNarrow = Expect<
  Equal<CommandExecutionItem["source"], "agent" | "userShell">
>;

export const publicSources = [
  "agent",
  "userShell",
  "unifiedExecStartup",
  "unifiedExecInteraction",
] satisfies CommandExecutionSource[];

export const liveSources = ["agent", "userShell"] satisfies CommandExecutionItem["source"][];

// @ts-expect-error the public source enum is closed.
export const rejectUnknownSource = "unknown" satisfies CommandExecutionSource;
// @ts-expect-error source values use exact camel-case spelling.
export const rejectWrongCaseSource = "UnifiedExecStartup" satisfies CommandExecutionSource;
// @ts-expect-error the source cannot be null.
export const rejectNullSource = null satisfies CommandExecutionSource;
// @ts-expect-error the source must be a string enum value.
export const rejectNumericSource = 1 satisfies CommandExecutionSource;
// @ts-expect-error standalone export does not broaden the live v1 item source.
export const rejectUnifiedExecOnLiveItem = "unifiedExecStartup" satisfies CommandExecutionItem["source"];

declare const publicSourceIsExact: PublicSourceIsExact;
declare const liveSourceRemainsNarrow: LiveSourceRemainsNarrow;
void publicSourceIsExact;
void liveSourceRemainsNarrow;
