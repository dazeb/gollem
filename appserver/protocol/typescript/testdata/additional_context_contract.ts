import type {
  AdditionalContextEntry,
  AdditionalContextKind,
  TurnStartParams,
  TurnSteerParams,
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

type Contracts = [
  Expect<Equal<AdditionalContextKind, "untrusted" | "application">>,
  Expect<Equal<AdditionalContextEntry, {
    kind: AdditionalContextKind;
    value: string;
  }>>,
];

export const kinds = ["untrusted", "application"] satisfies AdditionalContextKind[];
export const empty = { value: "", kind: "untrusted" } satisfies AdditionalContextEntry;
export const application = {
  value: "application context",
  kind: "application",
} satisfies AdditionalContextEntry;

// @ts-expect-error context kinds are closed.
export const rejectUnknownKind = "other" satisfies AdditionalContextKind;
// @ts-expect-error context kinds are non-null.
export const rejectNullKind = null satisfies AdditionalContextKind;
// @ts-expect-error value is required.
export const rejectMissingValue = { kind: "untrusted" } satisfies AdditionalContextEntry;
// @ts-expect-error value is non-null.
export const rejectNullValue = { value: null, kind: "untrusted" } satisfies AdditionalContextEntry;
// @ts-expect-error value is a string.
export const rejectNumericValue = { value: 1, kind: "untrusted" } satisfies AdditionalContextEntry;
// @ts-expect-error kind is required.
export const rejectMissingEntryKind = { value: "context" } satisfies AdditionalContextEntry;
// @ts-expect-error kind is non-null.
export const rejectNullEntryKind = { value: "context", kind: null } satisfies AdditionalContextEntry;
// @ts-expect-error entry kinds remain closed.
export const rejectEntryKind = { value: "context", kind: "other" } satisfies AdditionalContextEntry;
// @ts-expect-error generated entry TypeScript remains closed.
export const rejectUnknownEntryField = { value: "context", kind: "untrusted", future: true } satisfies AdditionalContextEntry;
// @ts-expect-error entries are objects.
export const rejectNullEntry = null satisfies AdditionalContextEntry;
// @ts-expect-error fixed turn/start params still exclude experimental additional context.
export const rejectTurnStartBinding = { threadId: "thread", input: [], additionalContext: {} } satisfies TurnStartParams;
// @ts-expect-error fixed turn/steer params still exclude experimental additional context.
export const rejectTurnSteerBinding = { threadId: "thread", input: [], expectedTurnId: "turn", additionalContext: {} } satisfies TurnSteerParams;

void (null as unknown as Contracts);
