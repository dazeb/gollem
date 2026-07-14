import type { LogoutAccountResponse } from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type ResponseContract = Expect<Equal<LogoutAccountResponse, Record<string, never>>>;

const response: LogoutAccountResponse = {};
void response;

// @ts-expect-error logout responses have no known fields.
const extra: LogoutAccountResponse = { accountId: "account" };
// @ts-expect-error logout responses are objects.
const nilResponse: LogoutAccountResponse = null;
// @ts-expect-error logout responses are not arrays.
const arrayResponse: LogoutAccountResponse = [];
// @ts-expect-error logout responses are not strings.
const stringResponse: LogoutAccountResponse = "";

void (null as unknown as ResponseContract);
void extra;
void nilResponse;
void arrayResponse;
void stringResponse;
