import type {
  ActivePermissionProfile,
  ApprovalsReviewer,
  AskForApproval,
  CollaborationMode,
  Personality,
  ReasoningEffort,
  ReasoningSummary,
  SandboxPolicy,
  ThreadExtra,
  ThreadHistoryMode,
  ThreadSettings,
  ThreadSettingsUpdatedNotification,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2) ? true : false;
type Expect<T extends true> = T;

type Contracts = [
  Expect<Equal<ThreadExtra, Record<string, never>>>,
  Expect<Equal<ThreadHistoryMode, "legacy" | "paginated">>,
  Expect<Equal<ThreadSettings, {
    activePermissionProfile?: ActivePermissionProfile | null;
    approvalPolicy: AskForApproval;
    approvalsReviewer: ApprovalsReviewer;
    collaborationMode: CollaborationMode;
    cwd: string;
    effort?: ReasoningEffort | null;
    model: string;
    modelProvider: string;
    personality?: Personality | null;
    sandboxPolicy: SandboxPolicy;
    serviceTier?: string | null;
    summary?: ReasoningSummary | null;
  }>>,
  Expect<Equal<ThreadSettingsUpdatedNotification, {
    threadId: string;
    threadSettings: ThreadSettings;
  }>>,
];

export const historyModes: ThreadHistoryMode[] = ["legacy", "paginated"];
export const extra: ThreadExtra = {};
export const settings = {
  cwd: "/workspace",
  approvalPolicy: "never",
  approvalsReviewer: "user",
  sandboxPolicy: { type: "dangerFullAccess" },
  model: "gpt-5",
  modelProvider: "openai",
  collaborationMode: {
    mode: "default",
    settings: {
      model: "gpt-5",
      reasoning_effort: null,
      developer_instructions: null,
    },
  },
} satisfies ThreadSettings;
export const notification: ThreadSettingsUpdatedNotification = {
  threadId: "thread",
  threadSettings: settings,
};

// @ts-expect-error history modes are a closed string union.
export const invalidHistoryMode: ThreadHistoryMode = "full";
// @ts-expect-error ThreadExtra is the empty upstream struct in TypeScript.
export const invalidExtra: ThreadExtra = { future: true };
// @ts-expect-error required settings fields cannot be omitted.
export const incompleteSettings: ThreadSettings = { model: "gpt-5" };
// @ts-expect-error nullable service tier remains a string when present.
export const invalidServiceTier: ThreadSettings = { ...settings, serviceTier: 1 };
// @ts-expect-error notification payload uses threadSettings, not a loose thread record.
export const invalidNotification: ThreadSettingsUpdatedNotification = { threadId: "thread", thread: settings };

void (null as unknown as Contracts);
