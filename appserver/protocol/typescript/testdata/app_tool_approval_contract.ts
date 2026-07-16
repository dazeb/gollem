import type {
  AppToolApproval,
  ItemPayloadByKind,
  MethodParamsByName,
  MethodResultsByName,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2) ? true : false;
type Expect<T extends true> = T;

type Contracts = [
  Expect<Equal<AppToolApproval, "auto" | "prompt" | "writes" | "approve">>,
  Expect<Equal<Extract<MethodParamsByName[keyof MethodParamsByName], AppToolApproval>, never>>,
  Expect<Equal<Extract<MethodResultsByName[keyof MethodResultsByName], AppToolApproval>, never>>,
  Expect<Equal<Extract<ItemPayloadByKind[keyof ItemPayloadByKind], AppToolApproval>, never>>,
];

["auto", "prompt", "writes", "approve"] satisfies AppToolApproval[];

// @ts-expect-error approvals are closed.
"other" satisfies AppToolApproval;
// @ts-expect-error exact lowercase spelling is required.
"AUTO" satisfies AppToolApproval;
// @ts-expect-error snake-case spelling is required.
"AutoApprove" satisfies AppToolApproval;
// @ts-expect-error whitespace is significant.
"writes " satisfies AppToolApproval;
// @ts-expect-error empty strings are not approvals.
"" satisfies AppToolApproval;
// @ts-expect-error approvals are non-null.
null satisfies AppToolApproval;
// @ts-expect-error approvals are strings.
1 satisfies AppToolApproval;
// @ts-expect-error approvals are strings.
true satisfies AppToolApproval;
// @ts-expect-error approvals are strings.
({}) satisfies AppToolApproval;

void (null as unknown as Contracts);
