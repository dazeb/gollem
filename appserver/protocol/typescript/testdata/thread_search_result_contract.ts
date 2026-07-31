import type {
  MethodResultsByName,
  Thread,
  ThreadSearchResult,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2) ? true : false;
type Expect<T extends true> = T;

type Contracts = [
  Expect<Equal<ThreadSearchResult, { snippet: string; thread: Thread }>>,
  Expect<Equal<"thread/search" extends keyof MethodResultsByName ? true : false, false>>,
];

({ thread: null as unknown as Thread, snippet: "matching text" }) satisfies ThreadSearchResult;

// @ts-expect-error thread is required.
({ snippet: "matching text" }) satisfies ThreadSearchResult;
// @ts-expect-error snippet is required.
({ thread: null as unknown as Thread }) satisfies ThreadSearchResult;

void (null as unknown as Contracts);
