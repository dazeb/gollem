import type {
  CommandAction,
  CommandExecutionAction,
  MethodParamsByName,
  MethodResultsByName,
  ParsedCommand,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2) ? true : false;
type Expect<T extends true> = T;

type ExactParsedCommand =
  | { type: "read"; cmd: string; name: string; path: string }
  | { type: "list_files"; cmd: string; path: string | null }
  | { type: "search"; cmd: string; query: string | null; path: string | null }
  | { type: "unknown"; cmd: string };

export type ParsedCommandContract = [
  Expect<Equal<ParsedCommand, ExactParsedCommand>>,
  Expect<Equal<Extract<ParsedCommand, CommandAction>, never>>,
  Expect<Equal<Extract<ParsedCommand, CommandExecutionAction>, never>>,
  Expect<Equal<Extract<MethodParamsByName[keyof MethodParamsByName], ParsedCommand>, never>>,
  Expect<Equal<Extract<MethodResultsByName[keyof MethodResultsByName], ParsedCommand>, never>>,
];

({ type: "read", cmd: "", name: "", path: "" }) satisfies ParsedCommand;
({ type: "read", cmd: "cat", name: "cat", path: "relative/../file" }) satisfies ParsedCommand;
({ type: "list_files", cmd: "ls", path: null }) satisfies ParsedCommand;
({ type: "list_files", cmd: "ls src", path: "relative/src" }) satisfies ParsedCommand;
({ type: "search", cmd: "rg", query: null, path: null }) satisfies ParsedCommand;
({ type: "search", cmd: "rg q", query: "q", path: "src" }) satisfies ParsedCommand;
({ type: "unknown", cmd: "custom --flag" }) satisfies ParsedCommand;

// @ts-expect-error read requires path.
({ type: "read", cmd: "cat", name: "cat" }) satisfies ParsedCommand;
// @ts-expect-error list_files path is canonical-required nullable.
({ type: "list_files", cmd: "ls" }) satisfies ParsedCommand;
// @ts-expect-error search query is canonical-required nullable.
({ type: "search", cmd: "rg", path: null }) satisfies ParsedCommand;
// @ts-expect-error search path is canonical-required nullable.
({ type: "search", cmd: "rg", query: null }) satisfies ParsedCommand;
// @ts-expect-error canonical variants are closed.
({ type: "unknown", cmd: "run", path: null }) satisfies ParsedCommand;
// @ts-expect-error discriminants are closed and snake-case.
({ type: "listFiles", cmd: "ls", path: null }) satisfies ParsedCommand;
// @ts-expect-error cmd is required.
({ type: "unknown" }) satisfies ParsedCommand;
