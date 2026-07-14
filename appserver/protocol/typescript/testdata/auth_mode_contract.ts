import type { AuthMode } from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? (<T>() => T extends B ? 1 : 2) extends
        (<T>() => T extends A ? 1 : 2)
      ? true
      : false
    : false;
type Expect<T extends true> = T;

type Contract = Expect<
  Equal<
    AuthMode,
    | "apikey"
    | "chatgpt"
    | "chatgptAuthTokens"
    | "headers"
    | "agentIdentity"
    | "personalAccessToken"
    | "bedrockApiKey"
  >
>;

export const modes = [
  "apikey",
  "chatgpt",
  "chatgptAuthTokens",
  "headers",
  "agentIdentity",
  "personalAccessToken",
  "bedrockApiKey",
] satisfies AuthMode[];

// @ts-expect-error auth modes are closed.
export const rejectUnknown = "other" satisfies AuthMode;
// @ts-expect-error the API-key mode is all lowercase.
export const rejectAPIKeyCase = "apiKey" satisfies AuthMode;
// @ts-expect-error exact ChatGPT casing is required.
export const rejectChatGPTCase = "ChatGPT" satisfies AuthMode;
// @ts-expect-error exact Bedrock API-key casing is required.
export const rejectBedrockCase = "bedrockapikey" satisfies AuthMode;
// @ts-expect-error empty strings are not auth modes.
export const rejectEmpty = "" satisfies AuthMode;
// @ts-expect-error auth modes are non-null.
export const rejectNull = null satisfies AuthMode;
// @ts-expect-error auth modes are strings.
export const rejectNumber = 1 satisfies AuthMode;

void (null as unknown as Contract);
