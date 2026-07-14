import type {
  ModelsRequirements,
  NewThreadModelDefaults,
  ReasoningEffort,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type Contracts = [
  Expect<Equal<NewThreadModelDefaults, {
    model: string | null;
    modelReasoningEffort: ReasoningEffort | null;
    serviceTier: string | null;
  }>>,
  Expect<Equal<ModelsRequirements, {
    newThread: NewThreadModelDefaults | null;
  }>>,
];

export const nullDefaults = {
  model: null,
  modelReasoningEffort: null,
  serviceTier: null,
} satisfies NewThreadModelDefaults;
export const fullDefaults = {
  model: "",
  modelReasoningEffort: "provider-effort",
  serviceTier: "",
} satisfies NewThreadModelDefaults;
export const nullRequirements = {
  newThread: null,
} satisfies ModelsRequirements;
export const fullRequirements = {
  newThread: fullDefaults,
} satisfies ModelsRequirements;

// @ts-expect-error model is required.
export const rejectMissingModel = { modelReasoningEffort: null, serviceTier: null } satisfies NewThreadModelDefaults;
// @ts-expect-error modelReasoningEffort is required.
export const rejectMissingEffort = { model: null, serviceTier: null } satisfies NewThreadModelDefaults;
// @ts-expect-error serviceTier is required.
export const rejectMissingTier = { model: null, modelReasoningEffort: null } satisfies NewThreadModelDefaults;
// @ts-expect-error model is nullable string only.
export const rejectNumericModel = { model: 1, modelReasoningEffort: null, serviceTier: null } satisfies NewThreadModelDefaults;
// @ts-expect-error reasoning effort is nullable string only.
export const rejectNumericEffort = { model: null, modelReasoningEffort: 1, serviceTier: null } satisfies NewThreadModelDefaults;
// @ts-expect-error exact defaults exclude extensions.
export const rejectDefaultsExtension = { model: null, modelReasoningEffort: null, serviceTier: null, extra: true } satisfies NewThreadModelDefaults;
// @ts-expect-error newThread is required.
export const rejectMissingNewThread = {} satisfies ModelsRequirements;
// @ts-expect-error newThread is nullable exact defaults only.
export const rejectWrongNewThread = { newThread: false } satisfies ModelsRequirements;
// @ts-expect-error exact requirements exclude extensions.
export const rejectRequirementsExtension = { newThread: null, extra: true } satisfies ModelsRequirements;

void (null as unknown as Contracts);
