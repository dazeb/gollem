import type {
  AttestationGenerateParams,
  DynamicToolCallParams,
  ServerRequest,
} from "../gollem_appserver_protocol";

export const dynamicParams = {
  threadId: "thread",
  turnId: "turn",
  callId: "call",
  namespace: null,
  tool: "client.search",
  arguments: null,
} satisfies DynamicToolCallParams;

export const dynamicRequest = {
  method: "item/tool/call",
  id: "request-1",
  params: dynamicParams,
} satisfies ServerRequest;

export const attestationRequest = {
  method: "attestation/generate",
  id: 42,
  params: {},
} satisfies ServerRequest;
attestationRequest.params satisfies AttestationGenerateParams;

// @ts-expect-error the method determines the required parameter type.
({ method: "item/tool/call", id: "request-1", params: {} }) satisfies ServerRequest;
// @ts-expect-error public server requests require an id.
({ method: "attestation/generate", params: {} }) satisfies ServerRequest;
// @ts-expect-error source variants are closed to known method names.
({ method: "unknown", id: "request-1", params: {} }) satisfies ServerRequest;
