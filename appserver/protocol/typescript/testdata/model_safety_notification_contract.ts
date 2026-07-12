import type {
  JsonValue,
  ModelRerouteReason,
  ModelReroutedNotification,
  ModelSafetyBufferingUpdatedNotification,
  ModelVerification,
  ModelVerificationNotification,
  TurnModerationMetadataNotification,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type Contracts = [
  Expect<Equal<ModelRerouteReason, "highRiskCyberActivity">>,
  Expect<Equal<ModelVerification, "trustedAccessForCyber">>,
  Expect<Equal<ModelReroutedNotification, {
    threadId: string;
    turnId: string;
    fromModel: string;
    toModel: string;
    reason: ModelRerouteReason;
  }>>,
  Expect<Equal<ModelVerificationNotification, {
    threadId: string;
    turnId: string;
    verifications: ModelVerification[];
  }>>,
  Expect<Equal<TurnModerationMetadataNotification, {
    threadId: string;
    turnId: string;
    metadata: JsonValue;
  }>>,
  Expect<Equal<ModelSafetyBufferingUpdatedNotification, {
    threadId: string;
    turnId: string;
    model: string;
    useCases: string[];
    reasons: string[];
    showBufferingUi: boolean;
    fasterModel: string | null;
  }>>,
];

export const rerouteReason: ModelRerouteReason = "highRiskCyberActivity";
export const verification: ModelVerification = "trustedAccessForCyber";
export const rerouted = {
  threadId: "thread",
  turnId: "turn",
  fromModel: "from",
  toModel: "to",
  reason: "highRiskCyberActivity",
} satisfies ModelReroutedNotification;
export const verifications = {
  threadId: "thread",
  turnId: "turn",
  verifications: ["trustedAccessForCyber"],
} satisfies ModelVerificationNotification;
export const moderation = {
  threadId: "thread",
  turnId: "turn",
  metadata: { score: 1, flags: [true, null] },
} satisfies TurnModerationMetadataNotification;
export const buffering = {
  threadId: "thread",
  turnId: "turn",
  model: "model",
  useCases: [],
  reasons: [],
  showBufferingUi: false,
  fasterModel: null,
} satisfies ModelSafetyBufferingUpdatedNotification;

// @ts-expect-error reroute reason is closed.
export const rejectRerouteReason: ModelRerouteReason = "other";
// @ts-expect-error verification is closed.
export const rejectVerification: ModelVerification = "other";
// @ts-expect-error rerouted notifications require reason.
export const rejectMissingReason = { threadId: "thread", turnId: "turn", fromModel: "from", toModel: "to" } satisfies ModelReroutedNotification;
// @ts-expect-error rerouted notification ids are non-null.
export const rejectNullRerouteThread = { threadId: null, turnId: "turn", fromModel: "from", toModel: "to", reason: "highRiskCyberActivity" } satisfies ModelReroutedNotification;
// @ts-expect-error rerouted notifications are closed.
export const rejectRerouteExtension = { threadId: "thread", turnId: "turn", fromModel: "from", toModel: "to", reason: "highRiskCyberActivity", at: "now" } satisfies ModelReroutedNotification;
// @ts-expect-error verification arrays are required.
export const rejectMissingVerifications = { threadId: "thread", turnId: "turn" } satisfies ModelVerificationNotification;
// @ts-expect-error verification arrays are non-null.
export const rejectNullVerifications = { threadId: "thread", turnId: "turn", verifications: null } satisfies ModelVerificationNotification;
// @ts-expect-error verification array values are closed.
export const rejectUnknownVerification = { threadId: "thread", turnId: "turn", verifications: ["other"] } satisfies ModelVerificationNotification;
// @ts-expect-error moderation metadata is required.
export const rejectMissingMetadata = { threadId: "thread", turnId: "turn" } satisfies TurnModerationMetadataNotification;
// @ts-expect-error undefined is not JSON metadata.
export const rejectUndefinedMetadata = { threadId: "thread", turnId: "turn", metadata: undefined } satisfies TurnModerationMetadataNotification;
// @ts-expect-error functions are not JSON metadata.
export const rejectFunctionMetadata = { threadId: "thread", turnId: "turn", metadata: () => true } satisfies TurnModerationMetadataNotification;
// @ts-expect-error useCases is required.
export const rejectMissingUseCases = { threadId: "thread", turnId: "turn", model: "model", reasons: [], showBufferingUi: false, fasterModel: null } satisfies ModelSafetyBufferingUpdatedNotification;
// @ts-expect-error reasons is required.
export const rejectMissingReasons = { threadId: "thread", turnId: "turn", model: "model", useCases: [], showBufferingUi: false, fasterModel: null } satisfies ModelSafetyBufferingUpdatedNotification;
// @ts-expect-error useCases is non-null.
export const rejectNullUseCases = { threadId: "thread", turnId: "turn", model: "model", useCases: null, reasons: [], showBufferingUi: false, fasterModel: null } satisfies ModelSafetyBufferingUpdatedNotification;
// @ts-expect-error reasons contain strings only.
export const rejectNumericReason = { threadId: "thread", turnId: "turn", model: "model", useCases: [], reasons: [1], showBufferingUi: false, fasterModel: null } satisfies ModelSafetyBufferingUpdatedNotification;
// @ts-expect-error UI flag is boolean.
export const rejectStringUiFlag = { threadId: "thread", turnId: "turn", model: "model", useCases: [], reasons: [], showBufferingUi: "false", fasterModel: null } satisfies ModelSafetyBufferingUpdatedNotification;
// @ts-expect-error canonical fasterModel is required even though nullable.
export const rejectMissingFasterModel = { threadId: "thread", turnId: "turn", model: "model", useCases: [], reasons: [], showBufferingUi: false } satisfies ModelSafetyBufferingUpdatedNotification;
// @ts-expect-error safety-buffering notifications are closed.
export const rejectBufferingExtension = { threadId: "thread", turnId: "turn", model: "model", useCases: [], reasons: [], showBufferingUi: false, fasterModel: null, at: "now" } satisfies ModelSafetyBufferingUpdatedNotification;

void (null as unknown as Contracts);
