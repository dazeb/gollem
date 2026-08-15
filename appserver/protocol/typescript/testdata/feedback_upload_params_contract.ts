import type { FeedbackUploadParams } from "../gollem_appserver_protocol";

export const feedback = {
  classification: "bug",
  reason: "details",
  threadId: null,
  includeLogs: true,
  extraLogFiles: ["relative.log", "/tmp/feedback"],
  tags: { source: "desktop" },
} satisfies FeedbackUploadParams;

// @ts-expect-error feedback classification is required.
({}) satisfies FeedbackUploadParams;
// @ts-expect-error source flags are booleans.
({ classification: "bug", includeLogs: "true" }) satisfies FeedbackUploadParams;
// @ts-expect-error feedback tags contain text values.
({ classification: "bug", tags: { source: 1 } }) satisfies FeedbackUploadParams;
