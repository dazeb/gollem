import type {
  AskForApproval,
  ComputerUseRequirements,
  ConfigRequirements,
  ConfigRequirementsReadResponse,
  ModelsRequirements,
  ResidencyRequirement,
  SandboxMode,
  WebSearchMode,
  WindowsSandboxSetupMode,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type Contracts = [
  Expect<Equal<ComputerUseRequirements, { allowLockedComputerUse: boolean | null }>>,
  Expect<Equal<ResidencyRequirement, "us">>,
  Expect<Equal<WebSearchMode, "disabled" | "cached" | "indexed" | "live">>,
  Expect<Equal<WindowsSandboxSetupMode, "elevated" | "unelevated">>,
  Expect<Equal<ConfigRequirements, {
    allowedApprovalPolicies: AskForApproval[] | null;
    allowedSandboxModes: SandboxMode[] | null;
    allowedWindowsSandboxImplementations: WindowsSandboxSetupMode[] | null;
    allowedPermissionProfiles: { [key in string]?: boolean } | null;
    defaultPermissions: string | null;
    allowedWebSearchModes: WebSearchMode[] | null;
    allowManagedHooksOnly: boolean | null;
    allowAppshots: boolean | null;
    allowRemoteControl: boolean | null;
    computerUse: ComputerUseRequirements | null;
    featureRequirements: { [key in string]?: boolean } | null;
    enforceResidency: ResidencyRequirement | null;
    models: ModelsRequirements | null;
  }>>,
  Expect<Equal<ConfigRequirementsReadResponse, { requirements: ConfigRequirements | null }>>,
];

export const nullComputerUse = {
  allowLockedComputerUse: null,
} satisfies ComputerUseRequirements;
export const fullComputerUse = {
  allowLockedComputerUse: false,
} satisfies ComputerUseRequirements;

export const nullRequirements = {
  allowedApprovalPolicies: null,
  allowedSandboxModes: null,
  allowedWindowsSandboxImplementations: null,
  allowedPermissionProfiles: null,
  defaultPermissions: null,
  allowedWebSearchModes: null,
  allowManagedHooksOnly: null,
  allowAppshots: null,
  allowRemoteControl: null,
  computerUse: null,
  featureRequirements: null,
  enforceResidency: null,
  models: null,
} satisfies ConfigRequirements;

export const fullRequirements = {
  allowedApprovalPolicies: ["untrusted"],
  allowedSandboxModes: ["read-only", "workspace-write"],
  allowedWindowsSandboxImplementations: ["elevated", "unelevated"],
  allowedPermissionProfiles: { "": false, strict: true },
  defaultPermissions: "",
  allowedWebSearchModes: ["disabled", "cached", "indexed", "live"],
  allowManagedHooksOnly: false,
  allowAppshots: true,
  allowRemoteControl: false,
  computerUse: fullComputerUse,
  featureRequirements: { "": false, feature: true },
  enforceResidency: "us",
  models: { newThread: null },
} satisfies ConfigRequirements;

export const nullResponse = {
  requirements: null,
} satisfies ConfigRequirementsReadResponse;
export const fullResponse = {
  requirements: fullRequirements,
} satisfies ConfigRequirementsReadResponse;

"us" satisfies ResidencyRequirement;
"disabled" satisfies WebSearchMode;
"cached" satisfies WebSearchMode;
"indexed" satisfies WebSearchMode;
"live" satisfies WebSearchMode;
"elevated" satisfies WindowsSandboxSetupMode;
"unelevated" satisfies WindowsSandboxSetupMode;

// @ts-expect-error allowLockedComputerUse is required.
export const rejectMissingComputerUse = {} satisfies ComputerUseRequirements;
// @ts-expect-error computer-use values are nullable booleans only.
export const rejectNumericComputerUse = { allowLockedComputerUse: 1 } satisfies ComputerUseRequirements;
// @ts-expect-error exact computer-use requirements exclude extensions.
export const rejectComputerUseExtension = { allowLockedComputerUse: null, extra: true } satisfies ComputerUseRequirements;
// @ts-expect-error residency values are closed.
"eu" satisfies ResidencyRequirement;
// @ts-expect-error web-search modes are closed.
"remote" satisfies WebSearchMode;
// @ts-expect-error Windows sandbox modes are closed.
"admin" satisfies WindowsSandboxSetupMode;
// @ts-expect-error all parent fields are required.
export const rejectMissingConfigFields = { models: null } satisfies ConfigRequirements;
// @ts-expect-error approval arrays exclude null members.
export const rejectNullApproval = { ...nullRequirements, allowedApprovalPolicies: [null] } satisfies ConfigRequirements;
// @ts-expect-error sandbox modes are closed.
export const rejectSandboxMode = { ...nullRequirements, allowedSandboxModes: ["readOnly"] } satisfies ConfigRequirements;
// @ts-expect-error Windows setup arrays exclude null members.
export const rejectNullWindowsMode = { ...nullRequirements, allowedWindowsSandboxImplementations: [null] } satisfies ConfigRequirements;
// @ts-expect-error permission-profile map values are booleans.
export const rejectPermissionMapValue = { ...nullRequirements, allowedPermissionProfiles: { strict: "true" } } satisfies ConfigRequirements;
// @ts-expect-error web-search arrays exclude unknown values.
export const rejectWebSearchMode = { ...nullRequirements, allowedWebSearchModes: ["remote"] } satisfies ConfigRequirements;
// @ts-expect-error nested computer-use requirements remain exact.
export const rejectNestedComputerUse = { ...nullRequirements, computerUse: {} } satisfies ConfigRequirements;
// @ts-expect-error feature map values are booleans.
export const rejectFeatureMapValue = { ...nullRequirements, featureRequirements: { feature: null } } satisfies ConfigRequirements;
// @ts-expect-error exact config requirements exclude extensions.
export const rejectConfigExtension = { ...nullRequirements, extra: true } satisfies ConfigRequirements;
// @ts-expect-error requirements is required nullable.
export const rejectMissingResponse = {} satisfies ConfigRequirementsReadResponse;
// @ts-expect-error response requirements use the exact parent.
export const rejectWrongResponse = { requirements: false } satisfies ConfigRequirementsReadResponse;
// @ts-expect-error exact responses exclude live data arrays.
export const rejectResponseExtension = { requirements: null, data: [] } satisfies ConfigRequirementsReadResponse;

void (null as unknown as Contracts);
