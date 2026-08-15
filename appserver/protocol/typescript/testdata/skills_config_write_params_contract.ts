import type { AbsolutePathBuf, SkillsConfigWriteParams } from "../gollem_appserver_protocol";

({ enabled: false }) satisfies SkillsConfigWriteParams;
({ path: "/workspace" as AbsolutePathBuf, name: "skill", enabled: true }) satisfies SkillsConfigWriteParams;
({ path: null, name: null, enabled: true }) satisfies SkillsConfigWriteParams;
// @ts-expect-error source requires enabled.
({ name: "skill" }) satisfies SkillsConfigWriteParams;
// @ts-expect-error source paths are strings.
({ path: 1, enabled: true }) satisfies SkillsConfigWriteParams;
