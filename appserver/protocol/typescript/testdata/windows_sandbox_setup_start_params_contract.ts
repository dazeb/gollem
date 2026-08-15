import type {
  WindowsSandboxSetupStartParams,
} from "../gollem_appserver_protocol";

export const sandboxSetup = {
  mode: "elevated",
  cwd: "/workspace/project",
} satisfies WindowsSandboxSetupStartParams;

// @ts-expect-error setup mode is required.
({}) satisfies WindowsSandboxSetupStartParams;
// @ts-expect-error setup mode is a closed source enum.
({ mode: "admin" }) satisfies WindowsSandboxSetupStartParams;
// @ts-expect-error setup cwd is a path string when supplied.
({ mode: "elevated", cwd: 1 }) satisfies WindowsSandboxSetupStartParams;
