import type {
  McpServerRefreshResponse,
  MethodParamsByName,
  MethodResultsByName,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type ResponseContract = Expect<Equal<McpServerRefreshResponse, Record<string, never>>>;
type ParamsRemainUnbound = Expect<Equal<
  "config/mcpServer/reload" extends keyof MethodParamsByName ? true : false,
  false
>>;
type ResultRemainsUnbound = Expect<Equal<
  "config/mcpServer/reload" extends keyof MethodResultsByName ? true : false,
  false
>>;

const response: McpServerRefreshResponse = {};
void response;

// @ts-expect-error refresh responses have no known fields.
const extra: McpServerRefreshResponse = { reloaded: false };
// @ts-expect-error refresh responses are objects.
const nilResponse: McpServerRefreshResponse = null;
// @ts-expect-error refresh responses are not arrays.
const arrayResponse: McpServerRefreshResponse = [];
// @ts-expect-error refresh responses are not strings.
const stringResponse: McpServerRefreshResponse = "";
// @ts-expect-error refresh responses are not numbers.
const numberResponse: McpServerRefreshResponse = 0;
// @ts-expect-error refresh responses are not booleans.
const booleanResponse: McpServerRefreshResponse = false;

void (null as unknown as ResponseContract);
void (null as unknown as ParamsRemainUnbound);
void (null as unknown as ResultRemainsUnbound);
void extra;
void nilResponse;
void arrayResponse;
void stringResponse;
void numberResponse;
void booleanResponse;
