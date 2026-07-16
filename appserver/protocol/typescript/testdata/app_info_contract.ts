import type {
  AppBranding,
  AppInfo,
  AppMetadata,
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

type ExactInfo = {
  id: string;
  name: string;
  description: string | null;
  logoUrl: string | null;
  logoUrlDark: string | null;
  iconAssets: { [key in string]?: string } | null;
  iconDarkAssets: { [key in string]?: string } | null;
  distributionChannel: string | null;
  branding: AppBranding | null;
  appMetadata: AppMetadata | null;
  labels: { [key in string]?: string } | null;
  installUrl: string | null;
  isAccessible: boolean;
  isEnabled: boolean;
  pluginDisplayNames: Array<string>;
};

type Contracts = [
  Expect<Equal<AppInfo, ExactInfo>>,
  ExpectFalse<"app/list" extends keyof MethodParamsByName ? true : false>,
  ExpectFalse<"app/list" extends keyof MethodResultsByName ? true : false>,
];

({
  id: "id",
  name: "name",
  description: null,
  logoUrl: null,
  logoUrlDark: null,
  iconAssets: null,
  iconDarkAssets: null,
  distributionChannel: null,
  branding: null,
  appMetadata: null,
  labels: null,
  installUrl: null,
  isAccessible: false,
  isEnabled: true,
  pluginDisplayNames: [],
}) satisfies AppInfo;

({
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
}) satisfies AppInfo;

// @ts-expect-error canonical AppInfo requires every field.
({ id: "id", name: "name" }) satisfies AppInfo;
// @ts-expect-error id is required.
({ name: "name", description: null, logoUrl: null, logoUrlDark: null, iconAssets: null, iconDarkAssets: null, distributionChannel: null, branding: null, appMetadata: null, labels: null, installUrl: null, isAccessible: false, isEnabled: true, pluginDisplayNames: [] }) satisfies AppInfo;
// @ts-expect-error name is a string.
({ id: "id", name: null, description: null, logoUrl: null, logoUrlDark: null, iconAssets: null, iconDarkAssets: null, distributionChannel: null, branding: null, appMetadata: null, labels: null, installUrl: null, isAccessible: false, isEnabled: true, pluginDisplayNames: [] }) satisfies AppInfo;
// @ts-expect-error description is a string or null.
({ id: "id", name: "name", description: 1, logoUrl: null, logoUrlDark: null, iconAssets: null, iconDarkAssets: null, distributionChannel: null, branding: null, appMetadata: null, labels: null, installUrl: null, isAccessible: false, isEnabled: true, pluginDisplayNames: [] }) satisfies AppInfo;
// @ts-expect-error iconAssets values are strings.
({ id: "id", name: "name", description: null, logoUrl: null, logoUrlDark: null, iconAssets: { key: null }, iconDarkAssets: null, distributionChannel: null, branding: null, appMetadata: null, labels: null, installUrl: null, isAccessible: false, isEnabled: true, pluginDisplayNames: [] }) satisfies AppInfo;
// @ts-expect-error iconDarkAssets is a string map or null.
({ id: "id", name: "name", description: null, logoUrl: null, logoUrlDark: null, iconAssets: null, iconDarkAssets: [], distributionChannel: null, branding: null, appMetadata: null, labels: null, installUrl: null, isAccessible: false, isEnabled: true, pluginDisplayNames: [] }) satisfies AppInfo;
// @ts-expect-error branding requires the canonical AppBranding fields.
({ id: "id", name: "name", description: null, logoUrl: null, logoUrlDark: null, iconAssets: null, iconDarkAssets: null, distributionChannel: null, branding: {}, appMetadata: null, labels: null, installUrl: null, isAccessible: false, isEnabled: true, pluginDisplayNames: [] }) satisfies AppInfo;
// @ts-expect-error appMetadata requires the canonical AppMetadata fields.
({ id: "id", name: "name", description: null, logoUrl: null, logoUrlDark: null, iconAssets: null, iconDarkAssets: null, distributionChannel: null, branding: null, appMetadata: {}, labels: null, installUrl: null, isAccessible: false, isEnabled: true, pluginDisplayNames: [] }) satisfies AppInfo;
// @ts-expect-error labels values are strings.
({ id: "id", name: "name", description: null, logoUrl: null, logoUrlDark: null, iconAssets: null, iconDarkAssets: null, distributionChannel: null, branding: null, appMetadata: null, labels: { key: false }, installUrl: null, isAccessible: false, isEnabled: true, pluginDisplayNames: [] }) satisfies AppInfo;
// @ts-expect-error isAccessible is boolean.
({ id: "id", name: "name", description: null, logoUrl: null, logoUrlDark: null, iconAssets: null, iconDarkAssets: null, distributionChannel: null, branding: null, appMetadata: null, labels: null, installUrl: null, isAccessible: null, isEnabled: true, pluginDisplayNames: [] }) satisfies AppInfo;
// @ts-expect-error isEnabled is boolean.
({ id: "id", name: "name", description: null, logoUrl: null, logoUrlDark: null, iconAssets: null, iconDarkAssets: null, distributionChannel: null, branding: null, appMetadata: null, labels: null, installUrl: null, isAccessible: false, isEnabled: "true", pluginDisplayNames: [] }) satisfies AppInfo;
// @ts-expect-error pluginDisplayNames is a string array.
({ id: "id", name: "name", description: null, logoUrl: null, logoUrlDark: null, iconAssets: null, iconDarkAssets: null, distributionChannel: null, branding: null, appMetadata: null, labels: null, installUrl: null, isAccessible: false, isEnabled: true, pluginDisplayNames: [null] }) satisfies AppInfo;
// @ts-expect-error canonical AppInfo is closed.
({ id: "id", name: "name", description: null, logoUrl: null, logoUrlDark: null, iconAssets: null, iconDarkAssets: null, distributionChannel: null, branding: null, appMetadata: null, labels: null, installUrl: null, isAccessible: false, isEnabled: true, pluginDisplayNames: [], future: true }) satisfies AppInfo;

void (null as unknown as Contracts);
