import type {
  AbsolutePathBuf,
  GuardianApprovalReviewAction,
  GuardianCommandSource,
  MethodParamsByName,
  MethodResultsByName,
  NetworkApprovalProtocol,
  RequestPermissionProfile,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type ExactAction =
  | { type: "command"; source: GuardianCommandSource; command: string; cwd: AbsolutePathBuf }
  | { type: "execve"; source: GuardianCommandSource; program: string; argv: Array<string>; cwd: AbsolutePathBuf }
  | { type: "applyPatch"; cwd: AbsolutePathBuf; files: Array<AbsolutePathBuf> }
  | { type: "networkAccess"; target: string; host: string; protocol: NetworkApprovalProtocol; port: number }
  | { type: "mcpToolCall"; server: string; toolName: string; connectorId: string | null; connectorName: string | null; toolTitle: string | null }
  | { type: "requestPermissions"; reason: string | null; permissions: RequestPermissionProfile };

type GuardianNotificationMethod =
  | "item/autoApprovalReview/started"
  | "item/autoApprovalReview/completed";

type Contracts = [
  Expect<Equal<GuardianApprovalReviewAction, ExactAction>>,
  Expect<Equal<Extract<keyof MethodParamsByName, GuardianNotificationMethod>, never>>,
  Expect<Equal<Extract<keyof MethodResultsByName, GuardianNotificationMethod>, never>>,
];

({ type: "command", source: "shell", command: "", cwd: "/workspace" }) satisfies GuardianApprovalReviewAction;
({ type: "execve", source: "unifiedExec", program: "", argv: ["-x", "-x"], cwd: "/workspace" }) satisfies GuardianApprovalReviewAction;
({ type: "applyPatch", cwd: "/workspace", files: [] }) satisfies GuardianApprovalReviewAction;
({ type: "networkAccess", target: "", host: "", protocol: "socks5Udp", port: 65535 }) satisfies GuardianApprovalReviewAction;
({ type: "mcpToolCall", server: "", toolName: "", connectorId: null, connectorName: "", toolTitle: null }) satisfies GuardianApprovalReviewAction;
({ type: "requestPermissions", reason: null, permissions: { network: null, fileSystem: null } }) satisfies GuardianApprovalReviewAction;

// @ts-expect-error action tags are closed.
({ type: "other" }) satisfies GuardianApprovalReviewAction;
// @ts-expect-error tags are case-sensitive.
({ type: "Command", source: "shell", command: "", cwd: "/workspace" }) satisfies GuardianApprovalReviewAction;
// @ts-expect-error command source is required.
({ type: "command", command: "", cwd: "/workspace" }) satisfies GuardianApprovalReviewAction;
// @ts-expect-error command sources are closed.
({ type: "command", source: "other", command: "", cwd: "/workspace" }) satisfies GuardianApprovalReviewAction;
// @ts-expect-error argv is required and non-null.
({ type: "execve", source: "shell", program: "p", argv: null, cwd: "/workspace" }) satisfies GuardianApprovalReviewAction;
// @ts-expect-error applyPatch files are required.
({ type: "applyPatch", cwd: "/workspace" }) satisfies GuardianApprovalReviewAction;
// @ts-expect-error network protocols are closed.
({ type: "networkAccess", target: "", host: "", protocol: "tcp", port: 1 }) satisfies GuardianApprovalReviewAction;
// @ts-expect-error network port is a number.
({ type: "networkAccess", target: "", host: "", protocol: "http", port: "1" }) satisfies GuardianApprovalReviewAction;
// @ts-expect-error canonical MCP actions require nullable connectorId.
({ type: "mcpToolCall", server: "", toolName: "", connectorName: null, toolTitle: null }) satisfies GuardianApprovalReviewAction;
// @ts-expect-error canonical MCP actions require nullable connectorName.
({ type: "mcpToolCall", server: "", toolName: "", connectorId: null, toolTitle: null }) satisfies GuardianApprovalReviewAction;
// @ts-expect-error canonical MCP actions require nullable toolTitle.
({ type: "mcpToolCall", server: "", toolName: "", connectorId: null, connectorName: null }) satisfies GuardianApprovalReviewAction;
// @ts-expect-error canonical permission actions require nullable reason.
({ type: "requestPermissions", permissions: { network: null, fileSystem: null } }) satisfies GuardianApprovalReviewAction;
// @ts-expect-error variants cannot cross fields.
({ type: "command", source: "shell", command: "", cwd: "/workspace", port: 1 }) satisfies GuardianApprovalReviewAction;

void (null as unknown as Contracts);
