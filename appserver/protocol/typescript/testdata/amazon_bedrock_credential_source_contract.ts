import type { AmazonBedrockCredentialSource } from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? (<T>() => T extends B ? 1 : 2) extends
        (<T>() => T extends A ? 1 : 2)
      ? true
      : false
    : false;
type Expect<T extends true> = T;

type Contract = Expect<Equal<AmazonBedrockCredentialSource, "codexManaged" | "awsManaged">>;

export const sources = ["codexManaged", "awsManaged"] satisfies AmazonBedrockCredentialSource[];

// @ts-expect-error credential sources are closed.
export const rejectUnknown = "other" satisfies AmazonBedrockCredentialSource;
// @ts-expect-error exact camel case is required.
export const rejectWrongCase = "awsmanaged" satisfies AmazonBedrockCredentialSource;
// @ts-expect-error empty strings are not credential sources.
export const rejectEmpty = "" satisfies AmazonBedrockCredentialSource;
// @ts-expect-error credential sources are non-null.
export const rejectNull = null satisfies AmazonBedrockCredentialSource;
// @ts-expect-error credential sources are strings.
export const rejectNumber = 1 satisfies AmazonBedrockCredentialSource;

void (null as unknown as Contract);
