import type {
  ItemPayloadByKind,
  LegacyAppPathString,
  MethodParamsByName,
  MethodResultsByName,
  TurnEnvironmentParams,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2) ? true : false;
type Expect<T extends true> = T;

type Contracts = [
  Expect<Equal<TurnEnvironmentParams, {
    cwd: LegacyAppPathString;
    environmentId: string;
    runtimeWorkspaceRoots?: Array<LegacyAppPathString> | null;
  }>>,
  Expect<Equal<Extract<MethodParamsByName[keyof MethodParamsByName], TurnEnvironmentParams>, never>>,
  Expect<Equal<Extract<MethodResultsByName[keyof MethodResultsByName], TurnEnvironmentParams>, never>>,
  Expect<Equal<Extract<ItemPayloadByKind[keyof ItemPayloadByKind], TurnEnvironmentParams>, never>>,
];

({ environmentId: "local", cwd: "/workspace", runtimeWorkspaceRoots: ["/workspace"] }) satisfies TurnEnvironmentParams;
({ environmentId: "local", cwd: "/workspace", runtimeWorkspaceRoots: null }) satisfies TurnEnvironmentParams;
({ environmentId: "local", cwd: "/workspace" }) satisfies TurnEnvironmentParams;

// @ts-expect-error environmentId is required.
({ cwd: "/workspace" }) satisfies TurnEnvironmentParams;
// @ts-expect-error roots must be strings.
({ environmentId: "local", cwd: "/workspace", runtimeWorkspaceRoots: [1] }) satisfies TurnEnvironmentParams;

void (null as unknown as Contracts);
