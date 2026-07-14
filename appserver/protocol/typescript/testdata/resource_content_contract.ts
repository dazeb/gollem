import type {
  JsonValue,
  MethodParamsByName,
  MethodResultsByName,
  ResourceContent,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type Expected =
  | { uri: string; mimeType?: string; text: string; _meta?: JsonValue }
  | { uri: string; mimeType?: string; blob: string; _meta?: JsonValue };
type Contracts = [
  Expect<Equal<ResourceContent, Expected>>,
  Expect<Equal<"mcpServer/resource/read" extends keyof MethodParamsByName ? true : false, false>>,
  Expect<Equal<"mcpServer/resource/read" extends keyof MethodResultsByName ? true : false, false>>,
];

export const text = { uri: "", text: "" } satisfies ResourceContent;
export const blob = { uri: "resource://blob", blob: "AA==" } satisfies ResourceContent;
export const fullText = {
  uri: "resource://text",
  mimeType: "text/plain",
  text: "hello",
  _meta: { source: ["client", null] },
} satisfies ResourceContent;
export const nullMeta = {
  uri: "resource://blob",
  blob: "AA==",
  _meta: null,
} satisfies ResourceContent;

// The exact public structural union accepts fields known by either variant.
export const crossed = {
  uri: "resource://crossed",
  text: "text",
  blob: "blob",
} satisfies ResourceContent;

// @ts-expect-error uri is required by both variants.
export const rejectMissingURI = { text: "value" } satisfies ResourceContent;
// @ts-expect-error one content field is required.
export const rejectMissingContent = { uri: "resource://missing" } satisfies ResourceContent;
// @ts-expect-error uri is non-null.
export const rejectNullURI = { uri: null, text: "value" } satisfies ResourceContent;
// @ts-expect-error text is non-null.
export const rejectNullText = { uri: "resource://text", text: null } satisfies ResourceContent;
// @ts-expect-error blob is non-null.
export const rejectNullBlob = { uri: "resource://blob", blob: null } satisfies ResourceContent;
// @ts-expect-error mimeType may be omitted but is non-null when present.
export const rejectNullMime = { uri: "resource://text", mimeType: null, text: "value" } satisfies ResourceContent;
// @ts-expect-error exact camelCase spelling is required.
export const rejectMimeAlias = { uri: "resource://text", mime_type: "text/plain", text: "value" } satisfies ResourceContent;
// @ts-expect-error fields absent from both public variants are rejected.
export const rejectExtra = { uri: "resource://text", text: "value", extra: true } satisfies ResourceContent;
// @ts-expect-error optional mimeType cannot be explicitly undefined.
export const rejectUndefinedMime = { uri: "resource://text", mimeType: undefined, text: "value" } satisfies ResourceContent;
// @ts-expect-error optional metadata cannot be explicitly undefined.
export const rejectUndefinedMeta = { uri: "resource://text", text: "value", _meta: undefined } satisfies ResourceContent;
// @ts-expect-error bigint is not a JSON value.
export const rejectBigIntMeta = { uri: "resource://text", text: "value", _meta: 1n } satisfies ResourceContent;
// @ts-expect-error functions are not JSON values.
export const rejectFunctionMeta = { uri: "resource://text", text: "value", _meta: () => true } satisfies ResourceContent;

void (null as unknown as Contracts);
