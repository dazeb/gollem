import type { AbsolutePathBuf, SkillsExtraRootsSetParams } from "../gollem_appserver_protocol";

({ extraRoots: [] }) satisfies SkillsExtraRootsSetParams;
({ extraRoots: ["/workspace" as AbsolutePathBuf] }) satisfies SkillsExtraRootsSetParams;
// @ts-expect-error source requires extraRoots.
({}) satisfies SkillsExtraRootsSetParams;
// @ts-expect-error source roots are paths.
({ extraRoots: [1] }) satisfies SkillsExtraRootsSetParams;
