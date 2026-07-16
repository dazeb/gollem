import type {
  AppTemplateSummary,
  AppTemplateUnavailableReason,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2) ? true : false;
type Expect<T extends true> = T;

type Contract = Expect<Equal<AppTemplateSummary, {
  canonicalConnectorId: string | null;
  category: string | null;
  description: string | null;
  logoUrl: string | null;
  logoUrlDark: string | null;
  materializedAppIds: Array<string>;
  name: string;
  reason: AppTemplateUnavailableReason | null;
  templateId: string;
}>>;

void (true satisfies Contract);

({
  templateId: "template",
  name: "name",
  description: null,
  category: null,
  canonicalConnectorId: null,
  logoUrl: null,
  logoUrlDark: null,
  materializedAppIds: [],
  reason: null,
}) satisfies AppTemplateSummary;

({
  templateId: " template ",
  name: "",
  description: " description ",
  category: " category ",
  canonicalConnectorId: " connector ",
  logoUrl: "not a url",
  logoUrlDark: "",
  materializedAppIds: ["app-2", "", "app-2", " app-1 "],
  reason: "NO_ACTIVE_WORKSPACE",
}) satisfies AppTemplateSummary;

// @ts-expect-error canonical AppTemplateSummary requires every field.
({ templateId: "template", name: "name", materializedAppIds: [] }) satisfies AppTemplateSummary;
// @ts-expect-error templateId is required.
({ name: "name", description: null, category: null, canonicalConnectorId: null, logoUrl: null, logoUrlDark: null, materializedAppIds: [], reason: null }) satisfies AppTemplateSummary;
// @ts-expect-error materializedAppIds cannot be null.
({ templateId: "template", name: "name", description: null, category: null, canonicalConnectorId: null, logoUrl: null, logoUrlDark: null, materializedAppIds: null, reason: null }) satisfies AppTemplateSummary;
// @ts-expect-error reason is the exact enum or null.
({ templateId: "template", name: "name", description: null, category: null, canonicalConnectorId: null, logoUrl: null, logoUrlDark: null, materializedAppIds: [], reason: "other" }) satisfies AppTemplateSummary;
// @ts-expect-error canonical AppTemplateSummary is closed.
({ templateId: "template", name: "name", description: null, category: null, canonicalConnectorId: null, logoUrl: null, logoUrlDark: null, materializedAppIds: [], reason: null, future: true }) satisfies AppTemplateSummary;
