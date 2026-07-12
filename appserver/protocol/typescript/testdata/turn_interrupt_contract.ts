import type {
  TurnInterruptParams,
  TurnInterruptResponse,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type Contracts = [
  Expect<Equal<TurnInterruptParams, { threadId: string; turnId: string }>>,
  Expect<Equal<TurnInterruptResponse, Record<string, never>>>,
];

export const emptyIds = { threadId: "", turnId: "" } satisfies TurnInterruptParams;
export const ids = { threadId: "thread", turnId: "turn" } satisfies TurnInterruptParams;
export const response = {} satisfies TurnInterruptResponse;

// @ts-expect-error threadId is required.
export const rejectMissingThread = { turnId: "turn" } satisfies TurnInterruptParams;
// @ts-expect-error turnId is required.
export const rejectMissingTurn = { threadId: "thread" } satisfies TurnInterruptParams;
// @ts-expect-error threadId is non-null.
export const rejectNullThread = { threadId: null, turnId: "turn" } satisfies TurnInterruptParams;
// @ts-expect-error turnId is non-null.
export const rejectNullTurn = { threadId: "thread", turnId: null } satisfies TurnInterruptParams;
// @ts-expect-error ids are strings.
export const rejectNumericTurn = { threadId: "thread", turnId: 1 } satisfies TurnInterruptParams;
// @ts-expect-error legacy id aliases are excluded.
export const rejectLegacyId = { threadId: "thread", turnId: "turn", id: "legacy" } satisfies TurnInterruptParams;
// @ts-expect-error the response is a closed empty object.
export const rejectRichResponse = { ok: true } satisfies TurnInterruptResponse;
// @ts-expect-error the empty response is non-null.
export const rejectNullResponse = null satisfies TurnInterruptResponse;

void (null as unknown as Contracts);
