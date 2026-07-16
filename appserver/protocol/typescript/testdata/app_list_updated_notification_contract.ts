import type {
  AppInfo,
  AppListUpdatedNotification,
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

type Contracts = [
  Expect<Equal<AppListUpdatedNotification, { data: Array<AppInfo> }>>,
  ExpectFalse<"app/list/updated" extends keyof MethodParamsByName ? true : false>,
  ExpectFalse<"app/list/updated" extends keyof MethodResultsByName ? true : false>,
];

({ data: [] }) satisfies AppListUpdatedNotification;

({
  data: [{
    id: " id ",
    name: "",
    description: " description ",
    logoUrl: "not a url",
    logoUrlDark: "",
    iconAssets: { "": "", " icon ": " value " },
    iconDarkAssets: {},
    distributionChannel: " arbitrary ",
    branding: {
      category: " category ",
      developer: "",
      website: "site",
      privacyPolicy: "privacy",
      termsOfService: "terms",
      isDiscoverableApp: true,
    },
    appMetadata: {
      review: { status: " review " },
      categories: ["", " category ", " category "],
      subCategories: [],
      seoDescription: " seo ",
      screenshots: [],
      developer: "dev",
      version: "v",
      versionId: "vid",
      versionNotes: "notes",
      firstPartyType: "first",
      firstPartyRequiresInstall: false,
      showInComposerWhenUnlinked: true,
    },
    labels: { "": "", " label ": " value " },
    installUrl: " install ",
    isAccessible: true,
    isEnabled: false,
    pluginDisplayNames: ["", " plugin ", " plugin "],
  }],
}) satisfies AppListUpdatedNotification;

// @ts-expect-error data is required.
({}) satisfies AppListUpdatedNotification;
// @ts-expect-error data is a non-null array.
({ data: null }) satisfies AppListUpdatedNotification;
// @ts-expect-error data is an array.
({ data: {} }) satisfies AppListUpdatedNotification;
// @ts-expect-error array items are AppInfo values.
({ data: [null] }) satisfies AppListUpdatedNotification;
// @ts-expect-error array items are AppInfo values.
({ data: [1] }) satisfies AppListUpdatedNotification;
// @ts-expect-error nested canonical AppInfo requires every field.
({ data: [{ id: "id", name: "name" }] }) satisfies AppListUpdatedNotification;
// @ts-expect-error nested pluginDisplayNames contains only strings.
({ data: [{ id: "id", name: "name", description: null, logoUrl: null, logoUrlDark: null, iconAssets: null, iconDarkAssets: null, distributionChannel: null, branding: null, appMetadata: null, labels: null, installUrl: null, isAccessible: false, isEnabled: true, pluginDisplayNames: [null] }] }) satisfies AppListUpdatedNotification;
// @ts-expect-error the canonical notification is closed.
({ data: [], future: true }) satisfies AppListUpdatedNotification;

void (null as unknown as Contracts);
