import type {
  AppInfo,
  AppsListParams,
  AppsListResponse,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2) ? true : false;
type Expect<T extends true> = T;

type Contracts = [
  Expect<Equal<AppsListParams, {
    cursor?: string | null;
    forceRefetch?: boolean;
    limit?: number | null;
    threadId?: string | null;
  }>>,
  Expect<Equal<AppsListResponse, {
    data: Array<AppInfo>;
    nextCursor: string | null;
  }>>,
];

({}) satisfies AppsListParams;
({ cursor: null, forceRefetch: false, limit: null, threadId: null }) satisfies AppsListParams;
({ cursor: "", forceRefetch: true, limit: 0, threadId: " " }) satisfies AppsListParams;
({ data: [], nextCursor: null }) satisfies AppsListResponse;

// @ts-expect-error cursor is nullable string only.
({ cursor: 1 }) satisfies AppsListParams;
// @ts-expect-error forceRefetch is optional but non-null.
({ forceRefetch: null }) satisfies AppsListParams;
// @ts-expect-error limit is nullable number only.
({ limit: "1" }) satisfies AppsListParams;
// @ts-expect-error threadId is nullable string only.
({ threadId: false }) satisfies AppsListParams;
// @ts-expect-error canonical compiler record is closed.
({ future: true }) satisfies AppsListParams;
// @ts-expect-error response requires data.
({ nextCursor: null }) satisfies AppsListResponse;
// @ts-expect-error response requires nextCursor in canonical TypeScript.
({ data: [] }) satisfies AppsListResponse;
// @ts-expect-error response data elements are strict AppInfo.
({ data: [{}], nextCursor: null }) satisfies AppsListResponse;
// @ts-expect-error response cursor is nullable string only.
({ data: [], nextCursor: 1 }) satisfies AppsListResponse;
// @ts-expect-error canonical response is closed.
({ data: [], nextCursor: null, future: true }) satisfies AppsListResponse;

void (null as unknown as Contracts);
