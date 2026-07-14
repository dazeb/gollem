import type {
  ModelProviderCapabilitiesReadParams,
  ModelProviderCapabilitiesReadResponse,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type Contracts = [
  Expect<Equal<ModelProviderCapabilitiesReadParams, Record<string, never>>>,
  Expect<Equal<ModelProviderCapabilitiesReadResponse, {
    namespaceTools: boolean;
    imageGeneration: boolean;
    webSearch: boolean;
  }>>,
];

export const emptyParams = {} satisfies ModelProviderCapabilitiesReadParams;
export const allFalse = {
  namespaceTools: false,
  imageGeneration: false,
  webSearch: false,
} satisfies ModelProviderCapabilitiesReadResponse;
export const allTrue = {
  namespaceTools: true,
  imageGeneration: true,
  webSearch: true,
} satisfies ModelProviderCapabilitiesReadResponse;
export const mixed = {
  namespaceTools: true,
  imageGeneration: false,
  webSearch: true,
} satisfies ModelProviderCapabilitiesReadResponse;

// @ts-expect-error exact params exclude provider selectors.
export const rejectProviderParam = { providerId: "openai" } satisfies ModelProviderCapabilitiesReadParams;
// @ts-expect-error namespaceTools is required.
export const rejectMissingNamespace = { imageGeneration: false, webSearch: false } satisfies ModelProviderCapabilitiesReadResponse;
// @ts-expect-error imageGeneration is required.
export const rejectMissingImage = { namespaceTools: false, webSearch: false } satisfies ModelProviderCapabilitiesReadResponse;
// @ts-expect-error webSearch is required.
export const rejectMissingWeb = { namespaceTools: false, imageGeneration: false } satisfies ModelProviderCapabilitiesReadResponse;
// @ts-expect-error capability values are non-null booleans.
export const rejectNullNamespace = { namespaceTools: null, imageGeneration: false, webSearch: false } satisfies ModelProviderCapabilitiesReadResponse;
// @ts-expect-error capability values are booleans only.
export const rejectStringImage = { namespaceTools: false, imageGeneration: "false", webSearch: false } satisfies ModelProviderCapabilitiesReadResponse;
// @ts-expect-error exact responses exclude broader Gollem capability metadata.
export const rejectConfigured = { namespaceTools: false, imageGeneration: false, webSearch: false, configured: false } satisfies ModelProviderCapabilitiesReadResponse;

void (null as unknown as Contracts);
