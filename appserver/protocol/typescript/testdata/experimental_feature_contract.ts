import type {
  ExperimentalFeature,
  ExperimentalFeatureEnablementSetParams,
  ExperimentalFeatureEnablementSetResponse,
  ExperimentalFeatureListParams,
  ExperimentalFeatureListResponse,
  ExperimentalFeatureStage,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2) ? true : false;
type Expect<T extends true> = T;

type Contracts = [
  Expect<Equal<ExperimentalFeatureStage, "beta" | "underDevelopment" | "stable" | "deprecated" | "removed">>,
  Expect<Equal<ExperimentalFeature, {
    announcement: string | null;
    defaultEnabled: boolean;
    description: string | null;
    displayName: string | null;
    enabled: boolean;
    name: string;
    stage: ExperimentalFeatureStage;
  }>>,
  Expect<Equal<ExperimentalFeatureListParams, {
    cursor?: string | null;
    limit?: number | null;
    threadId?: string | null;
  }>>,
  Expect<Equal<ExperimentalFeatureListResponse, {
    data: Array<ExperimentalFeature>;
    nextCursor: string | null;
  }>>,
  Expect<Equal<ExperimentalFeatureEnablementSetParams, { enablement: { [key: string]: boolean } }>>,
  Expect<Equal<ExperimentalFeatureEnablementSetResponse, { enablement: { [key: string]: boolean } }>>,
];

({ announcement: null, defaultEnabled: false, description: null, displayName: null, enabled: true, name: "", stage: "beta" }) satisfies ExperimentalFeature;
({}) satisfies ExperimentalFeatureListParams;
({ cursor: null, limit: 4294967295, threadId: null }) satisfies ExperimentalFeatureListParams;
({ data: [], nextCursor: null }) satisfies ExperimentalFeatureListResponse;
({ enablement: { "": true, feature: false } }) satisfies ExperimentalFeatureEnablementSetParams;

// @ts-expect-error stages are closed.
"unknown" satisfies ExperimentalFeatureStage;
// @ts-expect-error nullable display fields remain explicit.
({ defaultEnabled: false, enabled: true, name: "x", stage: "stable" }) satisfies ExperimentalFeature;
// @ts-expect-error limit is numeric rather than bigint.
({ limit: 1n }) satisfies ExperimentalFeatureListParams;
// @ts-expect-error data is required and non-null.
({ data: null, nextCursor: null }) satisfies ExperimentalFeatureListResponse;
// @ts-expect-error nextCursor remains explicit.
({ data: [] }) satisfies ExperimentalFeatureListResponse;
// @ts-expect-error map values are boolean.
({ enablement: { feature: "true" } }) satisfies ExperimentalFeatureEnablementSetResponse;

void (null as unknown as Contracts);
