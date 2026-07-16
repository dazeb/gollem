import type {
  FileChangeOutputDeltaNotification,
  MethodParamsByName,
  MethodResultsByName,
} from "../gollem_appserver_protocol.js";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type ExactNotification = {
  threadId: string;
  turnId: string;
  itemId: string;
  delta: string;
};

type Contracts = [
  Expect<Equal<FileChangeOutputDeltaNotification, ExactNotification>>,
  Expect<Equal<Extract<keyof MethodParamsByName, "item/fileChange/outputDelta">, never>>,
  Expect<Equal<Extract<keyof MethodResultsByName, "item/fileChange/outputDelta">, never>>,
];

({ threadId: "", turnId: "", itemId: "", delta: "" }) satisfies FileChangeOutputDeltaNotification;
({ threadId: "thread", turnId: "turn", itemId: "item", delta: "patch\nline" }) satisfies FileChangeOutputDeltaNotification;

// @ts-expect-error threadId is required.
({ turnId: "turn", itemId: "item", delta: "delta" }) satisfies FileChangeOutputDeltaNotification;
// @ts-expect-error turnId is required.
({ threadId: "thread", itemId: "item", delta: "delta" }) satisfies FileChangeOutputDeltaNotification;
// @ts-expect-error itemId is required.
({ threadId: "thread", turnId: "turn", delta: "delta" }) satisfies FileChangeOutputDeltaNotification;
// @ts-expect-error delta is required.
({ threadId: "thread", turnId: "turn", itemId: "item" }) satisfies FileChangeOutputDeltaNotification;
// @ts-expect-error ids are non-null strings.
({ threadId: null, turnId: "turn", itemId: "item", delta: "delta" }) satisfies FileChangeOutputDeltaNotification;
// @ts-expect-error delta is a string.
({ threadId: "thread", turnId: "turn", itemId: "item", delta: 1 }) satisfies FileChangeOutputDeltaNotification;
// @ts-expect-error canonical records are closed.
({ threadId: "thread", turnId: "turn", itemId: "item", delta: "delta", future: true }) satisfies FileChangeOutputDeltaNotification;
// @ts-expect-error values are non-null objects.
(null) satisfies FileChangeOutputDeltaNotification;
// @ts-expect-error values are not arrays.
([]) satisfies FileChangeOutputDeltaNotification;

void (null as unknown as Contracts);
