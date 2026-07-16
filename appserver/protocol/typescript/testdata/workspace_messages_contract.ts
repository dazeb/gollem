import type {
  GetWorkspaceMessagesResponse,
  WorkspaceMessage,
  WorkspaceMessageType,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2) ? true : false;
type Expect<T extends true> = T;

type Contracts = [
  Expect<Equal<WorkspaceMessageType, "headline" | "announcement" | "unknown">>,
  Expect<Equal<WorkspaceMessage, {
    archivedAt: number | null;
    createdAt: number | null;
    messageBody: string;
    messageId: string;
    messageType: WorkspaceMessageType;
  }>>,
  Expect<Equal<GetWorkspaceMessagesResponse, {
    featureEnabled: boolean;
    messages: Array<WorkspaceMessage>;
  }>>,
];

"headline" satisfies WorkspaceMessageType;
"unknown" satisfies WorkspaceMessageType;
({
  archivedAt: null,
  createdAt: null,
  messageBody: "",
  messageId: "",
  messageType: "announcement",
}) satisfies WorkspaceMessage;
({ featureEnabled: false, messages: [] }) satisfies GetWorkspaceMessagesResponse;

// @ts-expect-error message labels are exact.
"future" satisfies WorkspaceMessageType;
// @ts-expect-error nullable timestamps remain explicit.
({ messageId: "id", messageType: "headline", messageBody: "body" }) satisfies WorkspaceMessage;
// @ts-expect-error timestamps are emitted as number rather than bigint.
({ archivedAt: null, createdAt: 0n, messageBody: "body", messageId: "id", messageType: "headline" }) satisfies WorkspaceMessage;
// @ts-expect-error feature availability is required.
({ messages: [] }) satisfies GetWorkspaceMessagesResponse;
// @ts-expect-error messages are required and non-null.
({ featureEnabled: false, messages: null }) satisfies GetWorkspaceMessagesResponse;

void (null as unknown as Contracts);
