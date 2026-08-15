import type { PluginSkillReadParams } from "../gollem_appserver_protocol";

({ remoteMarketplaceName: "marketplace", remotePluginId: "plugin", skillName: "skill" }) satisfies PluginSkillReadParams;
// @ts-expect-error skillName is required.
({ remoteMarketplaceName: "marketplace", remotePluginId: "plugin" }) satisfies PluginSkillReadParams;
// @ts-expect-error remotePluginId is a string.
({ remoteMarketplaceName: "marketplace", remotePluginId: 1, skillName: "skill" }) satisfies PluginSkillReadParams;
