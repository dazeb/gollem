import type { SkillsListParams } from "../gollem_appserver_protocol";

export const skills = {
  cwds: ["relative", "/workspace"],
  forceReload: true,
} satisfies SkillsListParams;

// @ts-expect-error skills roots are text values.
({ cwds: [1] }) satisfies SkillsListParams;
// @ts-expect-error source flags are booleans.
({ forceReload: "true" }) satisfies SkillsListParams;
