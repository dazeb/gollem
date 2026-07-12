import type {
  Thread,
  ThreadListResponse,
  ThreadListResult,
  ThreadMetadataUpdateResponse,
  ThreadMetadataUpdateResult,
  ThreadReadResponse,
  ThreadReadResult,
  ThreadUnarchiveResponse,
  ThreadUnarchiveResult,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type Contracts = [
  Expect<Equal<ThreadListResponse, {
    backwardsCursor: string | null;
    data: Thread[];
    nextCursor: string | null;
  }>>,
  Expect<Equal<ThreadReadResponse, { thread: Thread }>>,
  Expect<Equal<ThreadMetadataUpdateResponse, { thread: Thread }>>,
  Expect<Equal<ThreadUnarchiveResponse, { thread: Thread }>>,
];

declare const thread: Thread;
export const list = {
  backwardsCursor: null,
  data: [thread],
  nextCursor: "",
} satisfies ThreadListResponse;
export const read = { thread } satisfies ThreadReadResponse;
export const metadata = { thread } satisfies ThreadMetadataUpdateResponse;
export const unarchive = { thread } satisfies ThreadUnarchiveResponse;

// @ts-expect-error cursors are required nullable.
export const missingCursor: ThreadListResponse = { data: [], nextCursor: null };
// @ts-expect-error data is a non-null array.
export const nullData: ThreadListResponse = { data: null, nextCursor: null, backwardsCursor: null };
// @ts-expect-error nested Thread remains strict.
export const malformedThread: ThreadReadResponse = { thread: { id: "thread" } };
// @ts-expect-error public responses exclude durable compatibility fields.
export const crossed: ThreadUnarchiveResponse = { thread, metadata: {} };

declare const liveList: ThreadListResult;
declare const liveRead: ThreadReadResult;
declare const liveMetadata: ThreadMetadataUpdateResult;
declare const liveUnarchive: ThreadUnarchiveResult;
// @ts-expect-error live durable list is not the exact public response.
export const publicList: ThreadListResponse = liveList;
// @ts-expect-error live durable read is not the exact public response.
export const publicRead: ThreadReadResponse = liveRead;
// @ts-expect-error live durable metadata result is not the exact public response.
export const publicMetadata: ThreadMetadataUpdateResponse = liveMetadata;
// @ts-expect-error live durable unarchive result is not the exact public response.
export const publicUnarchive: ThreadUnarchiveResponse = liveUnarchive;

void (null as unknown as Contracts);
