import type {
  TurnSteerParams,
  TurnSteerResponse,
  UserInput,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type ParamsExpected = {
  threadId: string;
  clientUserMessageId?: string | null;
  input: UserInput[];
  expectedTurnId: string;
};
type Contracts = [
  Expect<Equal<TurnSteerParams, ParamsExpected>>,
  Expect<Equal<TurnSteerResponse, { turnId: string }>>,
];

export const minimal = { threadId: "", input: [], expectedTurnId: "" } satisfies TurnSteerParams;
export const nullable = {
  threadId: "thread",
  clientUserMessageId: null,
  input: [],
  expectedTurnId: "turn",
} satisfies TurnSteerParams;
export const populated = {
  threadId: "thread",
  clientUserMessageId: "message",
  input: [{ type: "text", text: "hello", text_elements: [] }],
  expectedTurnId: "turn",
} satisfies TurnSteerParams;
export const response = { turnId: "" } satisfies TurnSteerResponse;

// @ts-expect-error threadId is required.
export const rejectMissingThread = { input: [], expectedTurnId: "turn" } satisfies TurnSteerParams;
// @ts-expect-error input is required.
export const rejectMissingInput = { threadId: "thread", expectedTurnId: "turn" } satisfies TurnSteerParams;
// @ts-expect-error expectedTurnId is required.
export const rejectMissingExpected = { threadId: "thread", input: [] } satisfies TurnSteerParams;
// @ts-expect-error input is non-null.
export const rejectNullInput = { threadId: "thread", input: null, expectedTurnId: "turn" } satisfies TurnSteerParams;
// @ts-expect-error nested input remains strict.
export const rejectMalformedInput = { threadId: "thread", input: [{ type: "text" }], expectedTurnId: "turn" } satisfies TurnSteerParams;
// @ts-expect-error live turnId aliases are excluded from the request.
export const rejectTurnId = { threadId: "thread", input: [], expectedTurnId: "turn", turnId: "turn" } satisfies TurnSteerParams;
// @ts-expect-error live prompt aliases are excluded.
export const rejectPrompt = { threadId: "thread", input: [], expectedTurnId: "turn", prompt: "hello" } satisfies TurnSteerParams;
// @ts-expect-error experimental metadata is filtered.
export const rejectMetadata = { threadId: "thread", input: [], expectedTurnId: "turn", responsesapiClientMetadata: {} } satisfies TurnSteerParams;
// @ts-expect-error response requires turnId.
export const rejectMissingResponseTurn = {} satisfies TurnSteerResponse;
// @ts-expect-error rich live response fields are excluded.
export const rejectAccepted = { turnId: "turn", accepted: true } satisfies TurnSteerResponse;

void (null as unknown as Contracts);
