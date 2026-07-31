import type {
  MethodParamsByName,
  MethodResultsByName,
  ThreadShellCommandParams,
  ThreadShellCommandResponse,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2) ? true : false;
type Expect<T extends true> = T;

type Contracts = [
  Expect<Equal<ThreadShellCommandParams, { command: string; threadId: string }>>,
  Expect<Equal<ThreadShellCommandResponse, Record<string, never>>>,
  Expect<Equal<"thread/shellCommand" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"thread/shellCommand" extends keyof MethodResultsByName ? true : false, false>>,
];

({ threadId: "thread", command: "printf 'hello' | sed 's/hello/world/'" }) satisfies ThreadShellCommandParams;
({}) satisfies ThreadShellCommandResponse;

// @ts-expect-error threadId is required.
({ command: "pwd" }) satisfies ThreadShellCommandParams;
// @ts-expect-error command is required.
({ threadId: "thread" }) satisfies ThreadShellCommandParams;
// @ts-expect-error the empty Rust response struct has no public fields.
({ future: true }) satisfies ThreadShellCommandResponse;

void (null as unknown as Contracts);
