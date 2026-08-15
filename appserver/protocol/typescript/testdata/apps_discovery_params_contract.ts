import type {
  AppsInstalledParams,
  AppsReadParams,
} from "../gollem_appserver_protocol";

export const installed = {
  threadId: null,
  forceRefresh: true,
} satisfies AppsInstalledParams;

export const read = {
  appIds: ["app-a", "app-b"],
  threadId: "thread",
  includeTools: true,
} satisfies AppsReadParams;

// @ts-expect-error app/read requires its app ids.
({}) satisfies AppsReadParams;
// @ts-expect-error app ids are text values.
({ appIds: [1] }) satisfies AppsReadParams;
// @ts-expect-error source flags are booleans.
({ forceRefresh: "true" }) satisfies AppsInstalledParams;
