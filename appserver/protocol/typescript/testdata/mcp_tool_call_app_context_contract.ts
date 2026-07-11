import type { McpToolCallAppContext } from "../gollem_appserver_protocol";

export const nullContext = {
  connectorId: "connector-1",
  linkId: null,
  resourceUri: null,
  appName: null,
  templateId: null,
  actionName: null,
} satisfies McpToolCallAppContext;

export const populatedContext = {
  connectorId: "connector-1",
  linkId: "link-1",
  resourceUri: "app://resource",
  appName: "Repo",
  templateId: "template-1",
  actionName: "search",
} satisfies McpToolCallAppContext;

// @ts-expect-error connectorId is required non-null.
export const rejectNullConnector = { ...nullContext, connectorId: null } satisfies McpToolCallAppContext;
// @ts-expect-error nullable fields are required, not optional.
export const rejectMissingLink = { connectorId: "connector-1", resourceUri: null, appName: null, templateId: null, actionName: null } satisfies McpToolCallAppContext;
// @ts-expect-error public fields use camelCase.
export const rejectSnakeCase = { connector_id: "connector-1", link_id: null, resource_uri: null, app_name: null, template_id: null, action_name: null } satisfies McpToolCallAppContext;
