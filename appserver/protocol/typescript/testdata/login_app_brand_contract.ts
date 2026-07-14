import type { LoginAppBrand } from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? (<T>() => T extends B ? 1 : 2) extends
        (<T>() => T extends A ? 1 : 2)
      ? true
      : false
    : false;
type Expect<T extends true> = T;

type Contract = Expect<Equal<LoginAppBrand, "codex" | "chatgpt">>;

export const brands = ["codex", "chatgpt"] satisfies LoginAppBrand[];

// @ts-expect-error login app brands are closed.
export const rejectUnknown = "other" satisfies LoginAppBrand;
// @ts-expect-error exact lowercase Codex spelling is required.
export const rejectCodexCase = "Codex" satisfies LoginAppBrand;
// @ts-expect-error exact lowercase ChatGPT spelling is required.
export const rejectChatGPTCase = "ChatGPT" satisfies LoginAppBrand;
// @ts-expect-error empty strings are not login app brands.
export const rejectEmpty = "" satisfies LoginAppBrand;
// @ts-expect-error login app brands are non-null.
export const rejectNull = null satisfies LoginAppBrand;
// @ts-expect-error login app brands are strings.
export const rejectNumber = 1 satisfies LoginAppBrand;

void (null as unknown as Contract);
