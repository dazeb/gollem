import type {
  Model,
  ModelListParams,
  ModelListResponse,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type ParamsExpected = {
  cursor?: string | null;
  limit?: number | null;
  includeHidden?: boolean | null;
};
type ResponseExpected = {
  data: Model[];
  nextCursor: string | null;
};
type Contracts = [
  Expect<Equal<ModelListParams, ParamsExpected>>,
  Expect<Equal<ModelListResponse, ResponseExpected>>,
];

export const emptyParams = {} satisfies ModelListParams;
export const nullParams = {
  cursor: null,
  limit: null,
  includeHidden: null,
} satisfies ModelListParams;
export const populatedParams = {
  cursor: "",
  limit: 0,
  includeHidden: false,
} satisfies ModelListParams;

declare const model: Model;
export const emptyResponse = {
  data: [],
  nextCursor: null,
} satisfies ModelListResponse;
export const populatedResponse = {
  data: [model, model],
  nextCursor: "",
} satisfies ModelListResponse;

// @ts-expect-error cursors are nullable strings only.
export const rejectNumericParamCursor = { cursor: 1 } satisfies ModelListParams;
// @ts-expect-error limits are nullable numbers only.
export const rejectStringLimit = { limit: "1" } satisfies ModelListParams;
// @ts-expect-error hidden selection is nullable boolean only.
export const rejectStringHidden = { includeHidden: "false" } satisfies ModelListParams;
// @ts-expect-error exact params exclude provider filters.
export const rejectProviderParam = { providerId: "openai" } satisfies ModelListParams;
// @ts-expect-error data is required.
export const rejectMissingData = { nextCursor: null } satisfies ModelListResponse;
// @ts-expect-error data is non-null.
export const rejectNullData = { data: null, nextCursor: null } satisfies ModelListResponse;
// @ts-expect-error data members are non-null exact models.
export const rejectNullModel = { data: [null], nextCursor: null } satisfies ModelListResponse;
// @ts-expect-error canonical TypeScript requires nextCursor.
export const rejectMissingNext = { data: [] } satisfies ModelListResponse;
// @ts-expect-error response cursors are nullable strings only.
export const rejectNumericNext = { data: [], nextCursor: 1 } satisfies ModelListResponse;
// @ts-expect-error nested models remain strict.
export const rejectMalformedModel = { data: [{}], nextCursor: null } satisfies ModelListResponse;
// @ts-expect-error exact responses exclude provider metadata.
export const rejectProviderResponse = { data: [], nextCursor: null, providerId: "openai" } satisfies ModelListResponse;

void (null as unknown as Contracts);
