import type { PluginUninstallParams } from "../gollem_appserver_protocol";

({ pluginId: "plugin-1" }) satisfies PluginUninstallParams;
// @ts-expect-error pluginId is required.
({}) satisfies PluginUninstallParams;
// @ts-expect-error pluginId is a string.
({ pluginId: 1 }) satisfies PluginUninstallParams;
