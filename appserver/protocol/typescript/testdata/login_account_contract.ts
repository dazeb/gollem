import type {
  LoginAccountParams,
  LoginAccountResponse,
  LoginAppBrand,
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

type ExpectedParams =
  | { type: "apiKey"; apiKey: string }
  | {
      type: "chatgpt";
      codexStreamlinedLogin?: boolean;
      useHostedLoginSuccessPage?: boolean;
      appBrand?: LoginAppBrand | null;
    }
  | { type: "chatgptDeviceCode" }
  | {
      type: "chatgptAuthTokens";
      accessToken: string;
      chatgptAccountId: string;
      chatgptPlanType?: string | null;
    }
  | { type: "amazonBedrock"; apiKey: string; region: string };

type ExpectedResponse =
  | { type: "apiKey" }
  | { type: "chatgpt"; loginId: string; authUrl: string }
  | {
      type: "chatgptDeviceCode";
      loginId: string;
      verificationUrl: string;
      userCode: string;
    }
  | { type: "chatgptAuthTokens" }
  | { type: "amazonBedrock" };

type ParamsContract = Expect<Equal<LoginAccountParams, ExpectedParams>>;
type ResponseContract = Expect<Equal<LoginAccountResponse, ExpectedResponse>>;

export const apiKey = { type: "apiKey", apiKey: "" } satisfies LoginAccountParams;
export const chatgpt = { type: "chatgpt" } satisfies LoginAccountParams;
export const chatgptFull = {
  type: "chatgpt",
  codexStreamlinedLogin: false,
  useHostedLoginSuccessPage: true,
  appBrand: null,
} satisfies LoginAccountParams;
export const deviceCode = { type: "chatgptDeviceCode" } satisfies LoginAccountParams;
export const authTokens = {
  type: "chatgptAuthTokens",
  accessToken: "",
  chatgptAccountId: "",
  chatgptPlanType: null,
} satisfies LoginAccountParams;
export const bedrock = {
  type: "amazonBedrock",
  apiKey: "",
  region: "",
} satisfies LoginAccountParams;

export const apiKeyResponse = { type: "apiKey" } satisfies LoginAccountResponse;
export const chatgptResponse = {
  type: "chatgpt",
  loginId: "",
  authUrl: "",
} satisfies LoginAccountResponse;
export const deviceCodeResponse = {
  type: "chatgptDeviceCode",
  loginId: "",
  verificationUrl: "",
  userCode: "",
} satisfies LoginAccountResponse;
export const authTokensResponse = { type: "chatgptAuthTokens" } satisfies LoginAccountResponse;
export const bedrockResponse = { type: "amazonBedrock" } satisfies LoginAccountResponse;

// @ts-expect-error request discriminants are closed and case-sensitive.
export const rejectRequestType = { type: "chatGPT" } satisfies LoginAccountParams;
// @ts-expect-error API key requests require apiKey.
export const rejectMissingApiKey = { type: "apiKey" } satisfies LoginAccountParams;
// @ts-expect-error API keys are non-null strings.
export const rejectNullApiKey = { type: "apiKey", apiKey: null } satisfies LoginAccountParams;
// @ts-expect-error ChatGPT booleans are non-null.
export const rejectNullBoolean = { type: "chatgpt", codexStreamlinedLogin: null } satisfies LoginAccountParams;
// @ts-expect-error appBrand is a closed enum.
export const rejectAppBrand = { type: "chatgpt", appBrand: "Codex" } satisfies LoginAccountParams;
// @ts-expect-error variant fields cannot cross into ChatGPT requests.
export const rejectCrossedChatgpt = { type: "chatgpt", apiKey: "key" } satisfies LoginAccountParams;
// @ts-expect-error device-code requests are closed.
export const rejectDeviceExtension = { type: "chatgptDeviceCode", appBrand: "codex" } satisfies LoginAccountParams;
// @ts-expect-error auth-token requests require an access token.
export const rejectMissingAccessToken = { type: "chatgptAuthTokens", chatgptAccountId: "account" } satisfies LoginAccountParams;
// @ts-expect-error account ids are non-null strings.
export const rejectNullAccountId = { type: "chatgptAuthTokens", accessToken: "token", chatgptAccountId: null } satisfies LoginAccountParams;
// @ts-expect-error plan type is a nullable string.
export const rejectNumericPlan = { type: "chatgptAuthTokens", accessToken: "token", chatgptAccountId: "account", chatgptPlanType: 1 } satisfies LoginAccountParams;
// @ts-expect-error Bedrock requests require region.
export const rejectMissingRegion = { type: "amazonBedrock", apiKey: "key" } satisfies LoginAccountParams;
// @ts-expect-error request variants are closed.
export const rejectRequestExtension = { type: "amazonBedrock", apiKey: "key", region: "region", extra: true } satisfies LoginAccountParams;

// @ts-expect-error response discriminants are closed and case-sensitive.
export const rejectResponseType = { type: "chatGPT" } satisfies LoginAccountResponse;
// @ts-expect-error ChatGPT responses require loginId.
export const rejectMissingLoginId = { type: "chatgpt", authUrl: "url" } satisfies LoginAccountResponse;
// @ts-expect-error authUrl is a non-null string.
export const rejectNullAuthUrl = { type: "chatgpt", loginId: "id", authUrl: null } satisfies LoginAccountResponse;
// @ts-expect-error device-code responses require userCode.
export const rejectMissingUserCode = { type: "chatgptDeviceCode", loginId: "id", verificationUrl: "url" } satisfies LoginAccountResponse;
// @ts-expect-error verificationUrl is a string.
export const rejectNumericVerificationUrl = { type: "chatgptDeviceCode", loginId: "id", verificationUrl: 1, userCode: "code" } satisfies LoginAccountResponse;
// @ts-expect-error empty response variants are closed.
export const rejectResponseExtension = { type: "chatgptAuthTokens", loginId: "id" } satisfies LoginAccountResponse;

void (null as unknown as ParamsContract);
void (null as unknown as ResponseContract);
