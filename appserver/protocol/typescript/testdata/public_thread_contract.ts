import type {
  AbsolutePathBuf,
  GitInfo,
  SessionSource,
  Thread,
  ThreadSource,
  ThreadStatus,
  Turn,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type ThreadIsExact = Expect<
  Equal<
    Thread,
    {
      id: string;
      sessionId: string;
      forkedFromId: string | null;
      parentThreadId: string | null;
      preview: string;
      ephemeral: boolean;
      modelProvider: string;
      createdAt: number;
      updatedAt: number;
      recencyAt: number | null;
      status: ThreadStatus;
      path: string | null;
      cwd: AbsolutePathBuf;
      cliVersion: string;
      source: SessionSource;
      threadSource: ThreadSource | null;
      agentNickname: string | null;
      agentRole: string | null;
      gitInfo: GitInfo | null;
      name: string | null;
      turns: Turn[];
    }
  >
>;

export const threads = [
  {
    id: "",
    sessionId: "",
    forkedFromId: null,
    parentThreadId: null,
    preview: "",
    ephemeral: false,
    modelProvider: "",
    createdAt: -1,
    updatedAt: 0,
    recencyAt: null,
    status: { type: "idle" },
    path: null,
    cwd: "/workspace",
    cliVersion: "",
    source: "cli",
    threadSource: null,
    agentNickname: null,
    agentRole: null,
    gitInfo: null,
    name: null,
    turns: [],
  },
] satisfies Thread[];

// @ts-expect-error nullable fields remain required
export const missingName: Thread = {
  id: "thread",
  sessionId: "session",
  forkedFromId: null,
  parentThreadId: null,
  preview: "",
  ephemeral: false,
  modelProvider: "openai",
  createdAt: 0,
  updatedAt: 0,
  recencyAt: null,
  status: { type: "idle" },
  path: null,
  cwd: "/workspace",
  cliVersion: "1",
  source: "cli",
  threadSource: null,
  agentNickname: null,
  agentRole: null,
  gitInfo: null,
  turns: [],
};

export const malformedThreads = [
  // @ts-expect-error turns cannot be null
  { ...threads[0], turns: null },
  // @ts-expect-error statuses remain strict
  { ...threads[0], status: { type: "active" } },
  // @ts-expect-error sources remain strict
  { ...threads[0], source: "mcp" },
  // @ts-expect-error experimental fields are excluded
  { ...threads[0], historyMode: "full" },
] satisfies Thread[];
