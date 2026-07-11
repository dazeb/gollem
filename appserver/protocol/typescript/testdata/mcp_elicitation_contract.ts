import type {
  McpElicitationArrayType,
  McpElicitationBooleanSchema,
  McpElicitationBooleanType,
  McpElicitationConstOption,
  McpElicitationEnumSchema,
  McpElicitationLegacyTitledEnumSchema,
  McpElicitationMultiSelectEnumSchema,
  McpElicitationNumberSchema,
  McpElicitationNumberType,
  McpElicitationObjectType,
  McpElicitationPrimitiveSchema,
  McpElicitationSchema,
  McpElicitationSingleSelectEnumSchema,
  McpElicitationStringFormat,
  McpElicitationStringSchema,
  McpElicitationStringType,
  McpElicitationTitledEnumItems,
  McpElicitationTitledMultiSelectEnumSchema,
  McpElicitationTitledSingleSelectEnumSchema,
  McpElicitationUntitledEnumItems,
  McpElicitationUntitledMultiSelectEnumSchema,
  McpElicitationUntitledSingleSelectEnumSchema,
  McpServerElicitationAction,
  McpServerElicitationRequestParams,
  McpServerElicitationRequestResponse,
  ResultFor,
} from "../gollem_appserver_protocol";

export const arrayType = "array" satisfies McpElicitationArrayType;
export const booleanType = "boolean" satisfies McpElicitationBooleanType;
export const numberTypes = ["number", "integer"] as const satisfies readonly McpElicitationNumberType[];
export const objectType = "object" satisfies McpElicitationObjectType;
export const stringType = "string" satisfies McpElicitationStringType;
export const formats = ["email", "uri", "date", "date-time"] as const satisfies readonly McpElicitationStringFormat[];
export const action = "accept" satisfies McpServerElicitationAction;

export const option = { const: "safe", title: "Safe" } satisfies McpElicitationConstOption;
export const stringSchema = {
  type: "string",
  minLength: 1,
  maxLength: 64,
  format: "email",
} satisfies McpElicitationStringSchema;
export const numberSchema = {
  type: "integer",
  minimum: 0,
  maximum: 10,
} satisfies McpElicitationNumberSchema;
export const booleanSchema = { type: "boolean", default: true } satisfies McpElicitationBooleanSchema;
export const legacyEnum = {
  type: "string",
  enum: ["safe", "fast"],
  enumNames: ["Safe", "Fast"],
} satisfies McpElicitationLegacyTitledEnumSchema;
export const untitledSingle = {
  type: "string",
  enum: ["safe", "fast"],
} satisfies McpElicitationUntitledSingleSelectEnumSchema;
export const titledSingle = {
  type: "string",
  oneOf: [option],
} satisfies McpElicitationTitledSingleSelectEnumSchema;
export const untitledItems = {
  type: "string",
  enum: ["read", "write"],
} satisfies McpElicitationUntitledEnumItems;
export const titledItems = { anyOf: [option] } satisfies McpElicitationTitledEnumItems;
export const untitledMulti = {
  type: "array",
  minItems: 1,
  maxItems: 2,
  items: untitledItems,
} satisfies McpElicitationUntitledMultiSelectEnumSchema;
export const titledMulti = {
  type: "array",
  items: titledItems,
} satisfies McpElicitationTitledMultiSelectEnumSchema;

untitledSingle satisfies McpElicitationSingleSelectEnumSchema;
titledSingle satisfies McpElicitationSingleSelectEnumSchema;
untitledMulti satisfies McpElicitationMultiSelectEnumSchema;
titledMulti satisfies McpElicitationMultiSelectEnumSchema;
legacyEnum satisfies McpElicitationEnumSchema;
stringSchema satisfies McpElicitationPrimitiveSchema;
numberSchema satisfies McpElicitationPrimitiveSchema;
booleanSchema satisfies McpElicitationPrimitiveSchema;

export const schema = {
  type: "object",
  properties: { email: stringSchema, count: numberSchema, enabled: booleanSchema },
  required: ["email"],
} satisfies McpElicitationSchema;

export const formRequest = {
  threadId: "thread-1",
  turnId: "turn-1",
  serverName: "repo",
  mode: "form",
  _meta: null,
  message: "Choose",
  requestedSchema: schema,
} satisfies McpServerElicitationRequestParams;
export const openAIFormRequest = {
  threadId: "thread-1",
  turnId: null,
  serverName: "repo",
  mode: "openai/form",
  _meta: null,
  message: "Choose",
  requestedSchema: { vendor: true },
} satisfies McpServerElicitationRequestParams;
export const urlRequest = {
  threadId: "thread-1",
  turnId: null,
  serverName: "repo",
  mode: "url",
  _meta: null,
  message: "Open",
  url: "https://example.com",
  elicitationId: "elicit-1",
} satisfies McpServerElicitationRequestParams;
export const response = {
  action,
  content: { email: "safe@example.com" },
  _meta: null,
} satisfies McpServerElicitationRequestResponse;
response satisfies ResultFor<"mcpServer/elicitation/request">;

// @ts-expect-error form mode requires the exact requested schema.
export const rejectFormWithoutSchema = { ...formRequest, requestedSchema: undefined } satisfies McpServerElicitationRequestParams;
// @ts-expect-error URL mode cannot carry a form schema.
export const rejectCrossedURL = { ...urlRequest, requestedSchema: schema } satisfies McpServerElicitationRequestParams;
// @ts-expect-error titled options require titles.
export const rejectUntitledConst = { const: "safe" } satisfies McpElicitationConstOption;
// @ts-expect-error multi-select item options use anyOf, not oneOf, in canonical output.
export const rejectCrossedItems = { oneOf: [option] } satisfies McpElicitationTitledEnumItems;
// @ts-expect-error responses require nullable content.
export const rejectMissingContent = { action: "cancel", _meta: null } satisfies McpServerElicitationRequestResponse;
// @ts-expect-error responses require nullable metadata.
export const rejectMissingMeta = { action: "cancel", content: null } satisfies McpServerElicitationRequestResponse;
