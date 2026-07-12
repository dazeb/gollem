import type {
  ConfigWarningNotification,
  TextPosition,
  TextRange,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type Contracts = [
  Expect<Equal<TextPosition, { line: number; column: number }>>,
  Expect<Equal<TextRange, { start: TextPosition; end: TextPosition }>>,
  Expect<Equal<ConfigWarningNotification, {
    summary: string;
    details: string | null;
    path?: string;
    range?: TextRange;
  }>>,
];

export const zeroPosition = { line: 0, column: 0 } satisfies TextPosition;
export const textRange = {
  start: { line: 1, column: 2 },
  end: { line: 3, column: 4 },
} satisfies TextRange;
export const minimalWarning = { summary: "", details: null } satisfies ConfigWarningNotification;
export const fullWarning = {
  summary: "warning",
  details: "fix it",
  path: "/tmp/config.toml",
  range: {
    start: { line: 1, column: 1 },
    end: { line: 2, column: 3 },
  },
} satisfies ConfigWarningNotification;

// @ts-expect-error positions require line.
export const rejectMissingLine = { column: 0 } satisfies TextPosition;
// @ts-expect-error positions require column.
export const rejectMissingColumn = { line: 0 } satisfies TextPosition;
// @ts-expect-error position coordinates are numbers.
export const rejectStringLine = { line: "1", column: 0 } satisfies TextPosition;
// @ts-expect-error positions are closed records.
export const rejectPositionExtension = { line: 0, column: 0, offset: 0 } satisfies TextPosition;
// @ts-expect-error ranges require end.
export const rejectMissingEnd = { start: { line: 0, column: 0 } } satisfies TextRange;
// @ts-expect-error nested positions remain strict.
export const rejectMalformedRange = { start: { line: 0, column: 0 }, end: { line: 0 } } satisfies TextRange;
// @ts-expect-error warning summary is required.
export const rejectMissingSummary = { details: null } satisfies ConfigWarningNotification;
// @ts-expect-error canonical warning details are required even though nullable.
export const rejectMissingDetails = { summary: "warning" } satisfies ConfigWarningNotification;
// @ts-expect-error warning details are nullable strings only.
export const rejectBooleanDetails = { summary: "warning", details: false } satisfies ConfigWarningNotification;
// @ts-expect-error optional path is non-null when present.
export const rejectNullPath = { summary: "warning", details: null, path: null } satisfies ConfigWarningNotification;
// @ts-expect-error optional range is non-null when present.
export const rejectNullRange = { summary: "warning", details: null, range: null } satisfies ConfigWarningNotification;
// @ts-expect-error warnings are closed records.
export const rejectWarningExtension = { summary: "warning", details: null, severity: "warning" } satisfies ConfigWarningNotification;

void (null as unknown as Contracts);
