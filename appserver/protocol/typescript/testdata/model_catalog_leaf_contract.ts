import type {
  InputModality,
  ModelAvailabilityNux,
  ModelServiceTier,
  ModelUpgradeInfo,
  ReasoningEffortOption,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type Contracts = [
  Expect<Equal<InputModality, "text" | "image">>,
  Expect<Equal<ReasoningEffortOption, {
    reasoningEffort: string;
    description: string;
  }>>,
  Expect<Equal<ModelAvailabilityNux, {
    message: string;
  }>>,
  Expect<Equal<ModelServiceTier, {
    id: string;
    name: string;
    description: string;
  }>>,
  Expect<Equal<ModelUpgradeInfo, {
    model: string;
    upgradeCopy: string | null;
    modelLink: string | null;
    migrationMarkdown: string | null;
  }>>,
];

export const text: InputModality = "text";
export const image: InputModality = "image";
export const effort = { reasoningEffort: "low", description: "" } satisfies ReasoningEffortOption;
export const nux = { message: "" } satisfies ModelAvailabilityNux;
export const tier = { id: "", name: "", description: "" } satisfies ModelServiceTier;
export const upgrade = {
  model: "next",
  upgradeCopy: null,
  modelLink: null,
  migrationMarkdown: null,
} satisfies ModelUpgradeInfo;

// @ts-expect-error input modalities are closed.
export const rejectAudio: InputModality = "audio";
// @ts-expect-error reasoning effort is required.
export const rejectMissingEffort = { description: "desc" } satisfies ReasoningEffortOption;
// @ts-expect-error reasoning description is required.
export const rejectMissingEffortDescription = { reasoningEffort: "low" } satisfies ReasoningEffortOption;
// @ts-expect-error reasoning effort is non-null.
export const rejectNullEffort = { reasoningEffort: null, description: "desc" } satisfies ReasoningEffortOption;
// @ts-expect-error reasoning effort options are closed records.
export const rejectEffortExtension = { reasoningEffort: "low", description: "desc", extra: true } satisfies ReasoningEffortOption;
// @ts-expect-error availability message is required.
export const rejectMissingMessage = {} satisfies ModelAvailabilityNux;
// @ts-expect-error availability message is non-null.
export const rejectNullMessage = { message: null } satisfies ModelAvailabilityNux;
// @ts-expect-error availability records are closed.
export const rejectNuxExtension = { message: "available", extra: true } satisfies ModelAvailabilityNux;
// @ts-expect-error service tier id is required.
export const rejectMissingTierId = { name: "tier", description: "desc" } satisfies ModelServiceTier;
// @ts-expect-error service tier name is non-null.
export const rejectNullTierName = { id: "id", name: null, description: "desc" } satisfies ModelServiceTier;
// @ts-expect-error service tier records are closed.
export const rejectTierExtension = { id: "id", name: "tier", description: "desc", extra: true } satisfies ModelServiceTier;
// @ts-expect-error upgrade model is required.
export const rejectMissingUpgradeModel = { upgradeCopy: null, modelLink: null, migrationMarkdown: null } satisfies ModelUpgradeInfo;
// @ts-expect-error canonical upgrade nullable fields are required.
export const rejectMissingUpgradeCopy = { model: "next", modelLink: null, migrationMarkdown: null } satisfies ModelUpgradeInfo;
// @ts-expect-error upgrade model is non-null.
export const rejectNullUpgradeModel = { model: null, upgradeCopy: null, modelLink: null, migrationMarkdown: null } satisfies ModelUpgradeInfo;
// @ts-expect-error upgrade links are nullable strings only.
export const rejectNumericUpgradeLink = { model: "next", upgradeCopy: null, modelLink: 1, migrationMarkdown: null } satisfies ModelUpgradeInfo;
// @ts-expect-error upgrade records are closed.
export const rejectUpgradeExtension = { model: "next", upgradeCopy: null, modelLink: null, migrationMarkdown: null, extra: true } satisfies ModelUpgradeInfo;

void (null as unknown as Contracts);
