import type {
  ProcessExitedNotification,
  ProcessOutputDeltaNotification,
  ProcessOutputStream,
  ProcessTerminalSize,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2) ? true : false;
type Expect<T extends true> = T;

type Contracts = [
  Expect<Equal<ProcessOutputStream, "stdout" | "stderr">>,
  Expect<Equal<ProcessTerminalSize, { rows: number; cols: number }>>,
  Expect<Equal<ProcessOutputDeltaNotification, {
    processHandle: string;
    stream: ProcessOutputStream;
    deltaBase64: string;
    capReached: boolean;
  }>>,
  Expect<Equal<ProcessExitedNotification, {
    processHandle: string;
    exitCode: number;
    stdout: string;
    stdoutCapReached: boolean;
    stderr: string;
    stderrCapReached: boolean;
  }>>,
];

"stdout" satisfies ProcessOutputStream;
"stderr" satisfies ProcessOutputStream;
({ rows: 0, cols: 65535 }) satisfies ProcessTerminalSize;
({ processHandle: "", stream: "stdout", deltaBase64: "not base64", capReached: false }) satisfies ProcessOutputDeltaNotification;
({ processHandle: "p", exitCode: -1, stdout: "", stdoutCapReached: true, stderr: "e", stderrCapReached: false }) satisfies ProcessExitedNotification;

// @ts-expect-error stream labels are exact.
"other" satisfies ProcessOutputStream;
// @ts-expect-error terminal rows are required.
({ cols: 80 }) satisfies ProcessTerminalSize;
// @ts-expect-error terminal dimensions are numeric.
({ rows: "24", cols: 80 }) satisfies ProcessTerminalSize;
// @ts-expect-error terminal size is closed.
({ rows: 24, cols: 80, future: true }) satisfies ProcessTerminalSize;
// @ts-expect-error output delta requires capReached.
({ processHandle: "p", stream: "stdout", deltaBase64: "" }) satisfies ProcessOutputDeltaNotification;
// @ts-expect-error output delta stream is exact.
({ processHandle: "p", stream: "other", deltaBase64: "", capReached: false }) satisfies ProcessOutputDeltaNotification;
// @ts-expect-error output delta is closed.
({ processHandle: "p", stream: "stdout", deltaBase64: "", capReached: false, future: true }) satisfies ProcessOutputDeltaNotification;
// @ts-expect-error exited requires stderrCapReached.
({ processHandle: "p", exitCode: 0, stdout: "", stdoutCapReached: false, stderr: "" }) satisfies ProcessExitedNotification;
// @ts-expect-error exited cap markers are boolean.
({ processHandle: "p", exitCode: 0, stdout: "", stdoutCapReached: 0, stderr: "", stderrCapReached: false }) satisfies ProcessExitedNotification;
// @ts-expect-error exited is closed.
({ processHandle: "p", exitCode: 0, stdout: "", stdoutCapReached: false, stderr: "", stderrCapReached: false, future: true }) satisfies ProcessExitedNotification;

void (null as unknown as Contracts);
