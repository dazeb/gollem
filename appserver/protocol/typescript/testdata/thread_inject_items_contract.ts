import type {
  JsonValue,
  MethodParamsByName,
  MethodResultsByName,
  ThreadInjectItemsParams,
  ThreadInjectItemsResponse,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2) ? true : false;
type Expect<T extends true> = T;

type Contracts = [
  Expect<Equal<ThreadInjectItemsParams, {
    items: Array<JsonValue>;
    threadId: string;
  }>>,
  Expect<Equal<ThreadInjectItemsResponse, Record<string, never>>>,
  Expect<Equal<"thread/inject_items" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"thread/inject_items" extends keyof MethodResultsByName ? true : false, false>>,
];

({ threadId: "thread", items: [null, true, 1, "text", { role: "user" }] }) satisfies ThreadInjectItemsParams;
({}) satisfies ThreadInjectItemsResponse;

// @ts-expect-error threadId is required.
({ items: [] }) satisfies ThreadInjectItemsParams;
// @ts-expect-error items are JSON values, not functions.
({ threadId: "thread", items: [() => undefined] }) satisfies ThreadInjectItemsParams;
// @ts-expect-error the empty Rust response struct has no public fields.
({ future: true }) satisfies ThreadInjectItemsResponse;

void (null as unknown as Contracts);
