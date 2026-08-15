import type {
  ThreadSectionAppearance,
  ThreadSectionCreateParams,
  ThreadSectionDeleteParams,
  ThreadSectionListParams,
  ThreadSectionMoveParams,
  ThreadSectionUpdateParams,
} from "../gollem_appserver_protocol";

export const appearance = {
  icon: "bookmark",
  color: null,
} satisfies ThreadSectionAppearance;

export const create = {
  name: "Pinned",
  appearance,
} satisfies ThreadSectionCreateParams;

export const remove = {
  sectionId: "section",
} satisfies ThreadSectionDeleteParams;

export const list = {} satisfies ThreadSectionListParams;

export const move = {
  threadId: "thread",
  sectionId: null,
} satisfies ThreadSectionMoveParams;

export const update = {
  sectionId: "section",
  name: "Pinned",
  appearance: null,
} satisfies ThreadSectionUpdateParams;

// @ts-expect-error the visual record carries both nullable source properties.
({ icon: "bookmark" }) satisfies ThreadSectionAppearance;
// @ts-expect-error creation requires a source name.
({}) satisfies ThreadSectionCreateParams;
// @ts-expect-error deletion requires a section identity.
({}) satisfies ThreadSectionDeleteParams;
// @ts-expect-error the list limit is an unsigned number, not text.
({ limit: "one" }) satisfies ThreadSectionListParams;
// @ts-expect-error move requires the nullable destination property.
({ threadId: "thread" }) satisfies ThreadSectionMoveParams;
// @ts-expect-error update requires the section identity and name.
({ sectionId: "section" }) satisfies ThreadSectionUpdateParams;
