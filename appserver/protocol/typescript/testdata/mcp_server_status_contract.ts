import type {
  JsonValue,
  ListMcpServerStatusResponse,
  McpAuthStatus,
  McpServerInfo,
  McpServerStatus,
  MethodParamsByName,
  MethodResultsByName,
  Resource,
  ResourceTemplate,
  Tool,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2) ? true : false;
type Expect<T extends true> = T;

type Contracts = [
  Expect<Equal<Resource, {
    annotations?: JsonValue;
    description?: string;
    mimeType?: string;
    name: string;
    size?: number;
    title?: string;
    uri: string;
    icons?: Array<JsonValue>;
    _meta?: JsonValue;
  }>>,
  Expect<Equal<ResourceTemplate, {
    annotations?: JsonValue;
    uriTemplate: string;
    name: string;
    title?: string;
    description?: string;
    mimeType?: string;
  }>>,
  Expect<Equal<Tool, {
    name: string;
    title?: string;
    description?: string;
    inputSchema: JsonValue;
    outputSchema?: JsonValue;
    annotations?: JsonValue;
    icons?: Array<JsonValue>;
    _meta?: JsonValue;
  }>>,
  Expect<Equal<McpServerStatus, {
    name: string;
    serverInfo: McpServerInfo | null;
    tools: { [key in string]?: Tool };
    resources: Array<Resource>;
    resourceTemplates: Array<ResourceTemplate>;
    authStatus: McpAuthStatus;
  }>>,
  Expect<Equal<ListMcpServerStatusResponse, {
    data: Array<McpServerStatus>;
    nextCursor: string | null;
  }>>,
  Expect<Equal<"mcpServerStatus/list" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"mcpServerStatus/list" extends keyof MethodResultsByName ? true : false, false>>,
];

({ name: "", uri: "" }) satisfies Resource;
({ name: "", uriTemplate: "" }) satisfies ResourceTemplate;
({ name: "", inputSchema: null }) satisfies Tool;
({ name: "", serverInfo: null, tools: {}, resources: [], resourceTemplates: [], authStatus: "unsupported" }) satisfies McpServerStatus;
({ data: [], nextCursor: null }) satisfies ListMcpServerStatusResponse;

// @ts-expect-error resource uri is required.
({ name: "" }) satisfies Resource;
// @ts-expect-error optional resource strings are non-null when present.
({ name: "", uri: "", mimeType: null }) satisfies Resource;
// @ts-expect-error the private snake-case adapter is not public.
({ name: "", uri_template: "" }) satisfies ResourceTemplate;
// @ts-expect-error inputSchema is required.
({ name: "" }) satisfies Tool;
// @ts-expect-error map values are strict tools.
({ name: "", serverInfo: null, tools: { bad: null }, resources: [], resourceTemplates: [], authStatus: "unsupported" }) satisfies McpServerStatus;
// @ts-expect-error serverInfo is explicit nullable.
({ name: "", tools: {}, resources: [], resourceTemplates: [], authStatus: "unsupported" }) satisfies McpServerStatus;
// @ts-expect-error nextCursor is explicit nullable.
({ data: [] }) satisfies ListMcpServerStatusResponse;

void (null as unknown as Contracts);
