import type {
  ConfiguredHookHandler,
  ConfiguredHookMatcherGroup,
  ManagedHooksRequirements,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? (<T>() => T extends B ? 1 : 2) extends
        (<T>() => T extends A ? 1 : 2)
      ? true
      : false
    : false;
type Expect<T extends true> = T;

type ManagedHooksRequirementsContract = [
  Expect<Equal<ConfiguredHookHandler,
    | {
        type: "command";
        command: string;
        commandWindows: string | null;
        timeoutSec: number | null;
        async: boolean;
        statusMessage: string | null;
      }
    | { type: "prompt" }
    | { type: "agent" }
  >>,
  Expect<Equal<ConfiguredHookMatcherGroup, {
    matcher: string | null;
    hooks: ConfiguredHookHandler[];
  }>>,
  Expect<Equal<ManagedHooksRequirements, {
    managedDir: string | null;
    windowsManagedDir: string | null;
    PreToolUse: ConfiguredHookMatcherGroup[];
    PermissionRequest: ConfiguredHookMatcherGroup[];
    PostToolUse: ConfiguredHookMatcherGroup[];
    PreCompact: ConfiguredHookMatcherGroup[];
    PostCompact: ConfiguredHookMatcherGroup[];
    SessionStart: ConfiguredHookMatcherGroup[];
    UserPromptSubmit: ConfiguredHookMatcherGroup[];
    SubagentStart: ConfiguredHookMatcherGroup[];
    SubagentStop: ConfiguredHookMatcherGroup[];
    Stop: ConfiguredHookMatcherGroup[];
  }>>,
];

[
  {
    type: "command",
    command: "",
    commandWindows: null,
    timeoutSec: null,
    async: false,
    statusMessage: null,
  },
  {
    type: "command",
    command: "run",
    commandWindows: "run.exe",
    timeoutSec: 0,
    async: true,
    statusMessage: "working",
  },
  { type: "prompt" },
  { type: "agent" },
] satisfies ConfiguredHookHandler[];

const emptyRequirements = {
  managedDir: null,
  windowsManagedDir: null,
  PreToolUse: [],
  PermissionRequest: [],
  PostToolUse: [],
  PreCompact: [],
  PostCompact: [],
  SessionStart: [],
  UserPromptSubmit: [],
  SubagentStart: [],
  SubagentStop: [],
  Stop: [],
} satisfies ManagedHooksRequirements;

({
  managedDir: "",
  windowsManagedDir: "C:\\hooks",
  PreToolUse: [{ matcher: "tool", hooks: [{ type: "prompt" }] }],
  PermissionRequest: [{ matcher: null, hooks: [{ type: "agent" }] }],
  PostToolUse: [],
  PreCompact: [],
  PostCompact: [],
  SessionStart: [],
  UserPromptSubmit: [],
  SubagentStart: [],
  SubagentStop: [],
  Stop: [],
}) satisfies ManagedHooksRequirements;

// @ts-expect-error command is required.
({ type: "command", commandWindows: null, timeoutSec: null, async: false, statusMessage: null }) satisfies ConfiguredHookHandler;
// @ts-expect-error async is required.
({ type: "command", command: "run", commandWindows: null, timeoutSec: null, statusMessage: null }) satisfies ConfiguredHookHandler;
// @ts-expect-error commandWindows is required nullable.
({ type: "command", command: "run", timeoutSec: null, async: false, statusMessage: null }) satisfies ConfiguredHookHandler;
// @ts-expect-error timeoutSec is required nullable.
({ type: "command", command: "run", commandWindows: null, async: false, statusMessage: null }) satisfies ConfiguredHookHandler;
// @ts-expect-error statusMessage is required nullable.
({ type: "command", command: "run", commandWindows: null, timeoutSec: null, async: false }) satisfies ConfiguredHookHandler;
// @ts-expect-error timeoutSec is a nullable number.
({ type: "command", command: "run", commandWindows: null, timeoutSec: "1", async: false, statusMessage: null }) satisfies ConfiguredHookHandler;
// @ts-expect-error prompt is a closed type-only variant.
({ type: "prompt", command: "run" }) satisfies ConfiguredHookHandler;
// @ts-expect-error agent is a closed type-only variant.
({ type: "agent", async: false }) satisfies ConfiguredHookHandler;
// @ts-expect-error handler discriminators are closed.
({ type: "other" }) satisfies ConfiguredHookHandler;
// @ts-expect-error matcher is required nullable.
({ hooks: [] }) satisfies ConfiguredHookMatcherGroup;
// @ts-expect-error hooks is required and non-null.
({ matcher: null, hooks: null }) satisfies ConfiguredHookMatcherGroup;
// @ts-expect-error hook arrays exclude null members.
({ matcher: null, hooks: [null] }) satisfies ConfiguredHookMatcherGroup;
// @ts-expect-error matcher is a nullable string.
({ matcher: 1, hooks: [] }) satisfies ConfiguredHookMatcherGroup;
// @ts-expect-error groups are closed.
({ matcher: null, hooks: [], extra: true }) satisfies ConfiguredHookMatcherGroup;
// @ts-expect-error every requirements field is required.
({ Stop: [] }) satisfies ManagedHooksRequirements;
// @ts-expect-error event keys are case-sensitive.
({ ...emptyRequirements, preToolUse: [] }) satisfies ManagedHooksRequirements;
// @ts-expect-error requirement arrays exclude null members.
({ ...emptyRequirements, PreToolUse: [null] }) satisfies ManagedHooksRequirements;
// @ts-expect-error managedDir is a nullable string.
({ ...emptyRequirements, managedDir: false }) satisfies ManagedHooksRequirements;
// @ts-expect-error nested handlers remain strict.
({ ...emptyRequirements, Stop: [{ matcher: null, hooks: [{ type: "other" }] }] }) satisfies ManagedHooksRequirements;
// @ts-expect-error requirements are closed.
({ ...emptyRequirements, extra: true }) satisfies ManagedHooksRequirements;

declare const contract: ManagedHooksRequirementsContract;
void contract;
