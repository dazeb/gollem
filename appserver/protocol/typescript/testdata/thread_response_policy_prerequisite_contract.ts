import type {
  AbsolutePathBuf,
  ApprovalsReviewer,
  AskForApproval,
  NetworkAccess,
  SandboxPolicy,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type Contracts = [
  Expect<Equal<ApprovalsReviewer, "user" | "auto_review" | "guardian_subagent">>,
  Expect<Equal<NetworkAccess, "restricted" | "enabled">>,
  Expect<Equal<AskForApproval,
    | "untrusted"
    | "on-request"
    | {
        granular: {
          sandbox_approval: boolean;
          rules: boolean;
          skill_approval: boolean;
          request_permissions: boolean;
          mcp_elicitations: boolean;
        };
      }
    | "never"
  >>,
  Expect<Equal<SandboxPolicy,
    | { type: "dangerFullAccess" }
    | { type: "readOnly"; networkAccess: boolean }
    | { type: "externalSandbox"; networkAccess: NetworkAccess }
    | {
        type: "workspaceWrite";
        writableRoots: AbsolutePathBuf[];
        networkAccess: boolean;
        excludeTmpdirEnvVar: boolean;
        excludeSlashTmp: boolean;
      }
  >>,
];

export const reviewers: ApprovalsReviewer[] = ["user", "auto_review", "guardian_subagent"];
export const access: NetworkAccess[] = ["restricted", "enabled"];
export const approvals: AskForApproval[] = [
  "untrusted",
  "on-request",
  "never",
  {
    granular: {
      sandbox_approval: true,
      rules: false,
      skill_approval: true,
      request_permissions: false,
      mcp_elicitations: true,
    },
  },
];
export const policies: SandboxPolicy[] = [
  { type: "dangerFullAccess" },
  { type: "readOnly", networkAccess: false },
  { type: "externalSandbox", networkAccess: "restricted" },
  {
    type: "workspaceWrite",
    writableRoots: ["/workspace"],
    networkAccess: true,
    excludeTmpdirEnvVar: false,
    excludeSlashTmp: true,
  },
];

// @ts-expect-error reviewer is a closed string union.
export const invalidReviewer: ApprovalsReviewer = "guardianSubagent";
// @ts-expect-error network access is a closed string union.
export const invalidAccess: NetworkAccess = "unrestricted";
export const incompleteGranular: AskForApproval = {
  // @ts-expect-error every granular boolean is required.
  granular: {
    sandbox_approval: true,
    rules: false,
    skill_approval: true,
    request_permissions: false,
  },
};
// @ts-expect-error fixed read-only output requires networkAccess.
export const defaultedReadOnly: SandboxPolicy = { type: "readOnly" };
// @ts-expect-error external sandbox uses the NetworkAccess enum, not boolean.
export const booleanExternal: SandboxPolicy = { type: "externalSandbox", networkAccess: false };
export const nullRoots: SandboxPolicy = {
  type: "workspaceWrite",
  // @ts-expect-error workspace-write roots are non-null.
  writableRoots: null,
  networkAccess: false,
  excludeTmpdirEnvVar: false,
  excludeSlashTmp: false,
};
// @ts-expect-error crossed fields are excluded from danger-full-access.
export const crossedPolicy: SandboxPolicy = { type: "dangerFullAccess", networkAccess: false };

void (null as unknown as Contracts);
