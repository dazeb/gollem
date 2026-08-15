import type { PluginShareListParams } from "../gollem_appserver_protocol";

({}) satisfies PluginShareListParams;
// @ts-expect-error source type permits no properties.
({ future: true }) satisfies PluginShareListParams;
