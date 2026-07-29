import type {
  MethodParamsByName,
  MethodResultsByName,
  PermissionProfileListParams,
  PermissionProfileListResponse,
  PermissionProfileSummary,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2) ? true : false;
type Expect<T extends true> = T;

type Contracts = [
  Expect<Equal<PermissionProfileListParams, {
    cursor?: string | null;
    cwd?: string | null;
    limit?: number | null;
  }>>,
  Expect<Equal<PermissionProfileSummary, {
    allowed: boolean;
    description: string | null;
    id: string;
  }>>,
  Expect<Equal<PermissionProfileListResponse, {
    data: Array<PermissionProfileSummary>;
    nextCursor: string | null;
  }>>,
  Expect<Equal<"permissionProfile/list" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"permissionProfile/list" extends keyof MethodResultsByName ? true : false, false>>,
];

({}) satisfies PermissionProfileListParams;
({ cursor: null, cwd: "", limit: 0 }) satisfies PermissionProfileListParams;
({ cursor: " next ", cwd: " /workspace ", limit: 4294967295 }) satisfies PermissionProfileListParams;
({ id: "", description: null, allowed: false }) satisfies PermissionProfileSummary;
({ id: "profile", description: "", allowed: true }) satisfies PermissionProfileSummary;
({ data: [], nextCursor: null }) satisfies PermissionProfileListResponse;
({ data: [{ id: "profile", description: null, allowed: true }], nextCursor: "next" }) satisfies PermissionProfileListResponse;

// @ts-expect-error limit is a number in pinned TypeScript, not bigint.
({ limit: 1n }) satisfies PermissionProfileListParams;
// @ts-expect-error cursor is nullable string data.
({ cursor: 1 }) satisfies PermissionProfileListParams;
// @ts-expect-error summary description is explicit nullable in TypeScript.
({ id: "profile", allowed: true }) satisfies PermissionProfileSummary;
// @ts-expect-error allowed is required.
({ id: "profile", description: null }) satisfies PermissionProfileSummary;
// @ts-expect-error response data is required.
({ nextCursor: null }) satisfies PermissionProfileListResponse;
// @ts-expect-error response nextCursor is explicit nullable in TypeScript.
({ data: [] }) satisfies PermissionProfileListResponse;
// @ts-expect-error summaries cannot be null.
({ data: [null], nextCursor: null }) satisfies PermissionProfileListResponse;

void (null as unknown as Contracts);
