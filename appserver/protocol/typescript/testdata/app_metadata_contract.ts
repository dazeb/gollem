import type {
  AppMetadata,
  AppReview,
  AppScreenshot,
  MethodParamsByName,
  MethodResultsByName,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;
type ExpectFalse<T extends false> = T;

type ExactMetadata = {
  review: AppReview | null;
  categories: Array<string> | null;
  subCategories: Array<string> | null;
  seoDescription: string | null;
  screenshots: Array<AppScreenshot> | null;
  developer: string | null;
  version: string | null;
  versionId: string | null;
  versionNotes: string | null;
  firstPartyType: string | null;
  firstPartyRequiresInstall: boolean | null;
  showInComposerWhenUnlinked: boolean | null;
};

type Contracts = [
  Expect<Equal<AppMetadata, ExactMetadata>>,
  ExpectFalse<"app/list" extends keyof MethodParamsByName ? true : false>,
  ExpectFalse<"app/list" extends keyof MethodResultsByName ? true : false>,
];

({
  review: null,
  categories: null,
  subCategories: null,
  seoDescription: null,
  screenshots: null,
  developer: null,
  version: null,
  versionId: null,
  versionNotes: null,
  firstPartyType: null,
  firstPartyRequiresInstall: null,
  showInComposerWhenUnlinked: null,
}) satisfies AppMetadata;

({
  review: { status: " arbitrary review " },
  categories: ["", " category ", " category "],
  subCategories: [],
  seoDescription: " seo ",
  screenshots: [
    { fileId: " file ", url: "not a url", userPrompt: " prompt " },
    { fileId: null, url: null, userPrompt: "" },
  ],
  developer: "",
  version: " v ",
  versionId: "id",
  versionNotes: " notes ",
  firstPartyType: " arbitrary ",
  firstPartyRequiresInstall: false,
  showInComposerWhenUnlinked: true,
}) satisfies AppMetadata;

// @ts-expect-error canonical metadata requires all nullable fields.
({}) satisfies AppMetadata;
// @ts-expect-error review is an AppReview or null.
({ review: "approved", categories: null, subCategories: null, seoDescription: null, screenshots: null, developer: null, version: null, versionId: null, versionNotes: null, firstPartyType: null, firstPartyRequiresInstall: null, showInComposerWhenUnlinked: null }) satisfies AppMetadata;
// @ts-expect-error review status is required.
({ review: {}, categories: null, subCategories: null, seoDescription: null, screenshots: null, developer: null, version: null, versionId: null, versionNotes: null, firstPartyType: null, firstPartyRequiresInstall: null, showInComposerWhenUnlinked: null }) satisfies AppMetadata;
// @ts-expect-error categories is a string array or null.
({ review: null, categories: "category", subCategories: null, seoDescription: null, screenshots: null, developer: null, version: null, versionId: null, versionNotes: null, firstPartyType: null, firstPartyRequiresInstall: null, showInComposerWhenUnlinked: null }) satisfies AppMetadata;
// @ts-expect-error category elements are strings.
({ review: null, categories: [null], subCategories: null, seoDescription: null, screenshots: null, developer: null, version: null, versionId: null, versionNotes: null, firstPartyType: null, firstPartyRequiresInstall: null, showInComposerWhenUnlinked: null }) satisfies AppMetadata;
// @ts-expect-error subCategories is a string array or null.
({ review: null, categories: null, subCategories: [false], seoDescription: null, screenshots: null, developer: null, version: null, versionId: null, versionNotes: null, firstPartyType: null, firstPartyRequiresInstall: null, showInComposerWhenUnlinked: null }) satisfies AppMetadata;
// @ts-expect-error seoDescription is a string or null.
({ review: null, categories: null, subCategories: null, seoDescription: 1, screenshots: null, developer: null, version: null, versionId: null, versionNotes: null, firstPartyType: null, firstPartyRequiresInstall: null, showInComposerWhenUnlinked: null }) satisfies AppMetadata;
// @ts-expect-error screenshots is an AppScreenshot array or null.
({ review: null, categories: null, subCategories: null, seoDescription: null, screenshots: {}, developer: null, version: null, versionId: null, versionNotes: null, firstPartyType: null, firstPartyRequiresInstall: null, showInComposerWhenUnlinked: null }) satisfies AppMetadata;
// @ts-expect-error screenshot elements cannot be null.
({ review: null, categories: null, subCategories: null, seoDescription: null, screenshots: [null], developer: null, version: null, versionId: null, versionNotes: null, firstPartyType: null, firstPartyRequiresInstall: null, showInComposerWhenUnlinked: null }) satisfies AppMetadata;
// @ts-expect-error nested screenshots require canonical nullable fields.
({ review: null, categories: null, subCategories: null, seoDescription: null, screenshots: [{ userPrompt: "x" }], developer: null, version: null, versionId: null, versionNotes: null, firstPartyType: null, firstPartyRequiresInstall: null, showInComposerWhenUnlinked: null }) satisfies AppMetadata;
// @ts-expect-error developer is a string or null.
({ review: null, categories: null, subCategories: null, seoDescription: null, screenshots: null, developer: false, version: null, versionId: null, versionNotes: null, firstPartyType: null, firstPartyRequiresInstall: null, showInComposerWhenUnlinked: null }) satisfies AppMetadata;
// @ts-expect-error version is a string or null.
({ review: null, categories: null, subCategories: null, seoDescription: null, screenshots: null, developer: null, version: 1, versionId: null, versionNotes: null, firstPartyType: null, firstPartyRequiresInstall: null, showInComposerWhenUnlinked: null }) satisfies AppMetadata;
// @ts-expect-error firstPartyRequiresInstall is boolean or null.
({ review: null, categories: null, subCategories: null, seoDescription: null, screenshots: null, developer: null, version: null, versionId: null, versionNotes: null, firstPartyType: null, firstPartyRequiresInstall: "false", showInComposerWhenUnlinked: null }) satisfies AppMetadata;
// @ts-expect-error showInComposerWhenUnlinked is boolean or null.
({ review: null, categories: null, subCategories: null, seoDescription: null, screenshots: null, developer: null, version: null, versionId: null, versionNotes: null, firstPartyType: null, firstPartyRequiresInstall: null, showInComposerWhenUnlinked: 0 }) satisfies AppMetadata;
// @ts-expect-error canonical metadata is closed.
({ review: null, categories: null, subCategories: null, seoDescription: null, screenshots: null, developer: null, version: null, versionId: null, versionNotes: null, firstPartyType: null, firstPartyRequiresInstall: null, showInComposerWhenUnlinked: null, future: true }) satisfies AppMetadata;

void (null as unknown as Contracts);
