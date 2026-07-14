import type { Model } from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type Contract = Expect<Equal<Model, {
  id: string;
  model: string;
  upgrade: string | null;
  upgradeInfo: import("../gollem_appserver_protocol").ModelUpgradeInfo | null;
  availabilityNux: import("../gollem_appserver_protocol").ModelAvailabilityNux | null;
  displayName: string;
  description: string;
  hidden: boolean;
  supportedReasoningEfforts: Array<import("../gollem_appserver_protocol").ReasoningEffortOption>;
  defaultReasoningEffort: import("../gollem_appserver_protocol").ReasoningEffort;
  inputModalities: Array<import("../gollem_appserver_protocol").InputModality>;
  supportsPersonality: boolean;
  additionalSpeedTiers: Array<string>;
  serviceTiers: Array<import("../gollem_appserver_protocol").ModelServiceTier>;
  defaultServiceTier: string | null;
  isDefault: boolean;
}>>;

export const model = {
  id: "gpt",
  model: "gpt",
  upgrade: null,
  upgradeInfo: null,
  availabilityNux: null,
  displayName: "GPT",
  description: "",
  hidden: false,
  supportedReasoningEfforts: [{ reasoningEffort: "low", description: "" }],
  defaultReasoningEffort: "low",
  inputModalities: ["text", "image"],
  supportsPersonality: false,
  additionalSpeedTiers: [],
  serviceTiers: [],
  defaultServiceTier: null,
  isDefault: true,
} satisfies Model;

// @ts-expect-error id is required.
export const rejectMissingId = { ...model, id: undefined } satisfies Model;
// @ts-expect-error canonical nullable upgrade is required.
export const rejectMissingUpgrade = (({ upgrade: _, ...rest }) => rest)(model) satisfies Model;
// @ts-expect-error canonical input modalities are required.
export const rejectMissingModalities = (({ inputModalities: _, ...rest }) => rest)(model) satisfies Model;
// @ts-expect-error canonical default service tier is required.
export const rejectMissingDefaultTier = (({ defaultServiceTier: _, ...rest }) => rest)(model) satisfies Model;
// @ts-expect-error ids are non-null strings.
export const rejectNullId = { ...model, id: null } satisfies Model;
// @ts-expect-error reasoning effort arrays are non-null.
export const rejectNullEfforts = { ...model, supportedReasoningEfforts: null } satisfies Model;
// @ts-expect-error reasoning effort array elements are non-null.
export const rejectNullEffort = { ...model, supportedReasoningEfforts: [null] } satisfies Model;
// @ts-expect-error reasoning effort descriptions are required.
export const rejectBadEffort = { ...model, supportedReasoningEfforts: [{ reasoningEffort: "low" }] } satisfies Model;
// @ts-expect-error input modalities are closed.
export const rejectAudio = { ...model, inputModalities: ["audio"] } satisfies Model;
// @ts-expect-error default reasoning effort is non-null.
export const rejectNullDefaultEffort = { ...model, defaultReasoningEffort: null } satisfies Model;
// @ts-expect-error service tiers require descriptions.
export const rejectBadTier = { ...model, serviceTiers: [{ id: "id", name: "name" }] } satisfies Model;
// @ts-expect-error personality support is boolean.
export const rejectStringPersonality = { ...model, supportsPersonality: "false" } satisfies Model;
// @ts-expect-error isDefault is required.
export const rejectMissingDefault = (({ isDefault: _, ...rest }) => rest)(model) satisfies Model;
// @ts-expect-error model records are closed.
export const rejectExtension = { ...model, providerId: "openai" } satisfies Model;

void (null as unknown as Contract);
