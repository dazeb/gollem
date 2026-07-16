import type {
  AppBranding,
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

type ExactBranding = {
  category: string | null;
  developer: string | null;
  isDiscoverableApp: boolean;
  privacyPolicy: string | null;
  termsOfService: string | null;
  website: string | null;
};
type ExactReview = { status: string };
type ExactScreenshot = {
  fileId: string | null;
  url: string | null;
  userPrompt: string;
};

type Contracts = [
  Expect<Equal<AppBranding, ExactBranding>>,
  Expect<Equal<AppReview, ExactReview>>,
  Expect<Equal<AppScreenshot, ExactScreenshot>>,
  ExpectFalse<"app/list" extends keyof MethodParamsByName ? true : false>,
  ExpectFalse<"app/list" extends keyof MethodResultsByName ? true : false>,
];

({ category: null, developer: null, isDiscoverableApp: false, privacyPolicy: null, termsOfService: null, website: null }) satisfies AppBranding;
({ category: "", developer: " arbitrary ", isDiscoverableApp: true, privacyPolicy: "", termsOfService: "", website: "not a url" }) satisfies AppBranding;
({ status: "" }) satisfies AppReview;
({ status: " arbitrary " }) satisfies AppReview;
({ fileId: null, url: null, userPrompt: "" }) satisfies AppScreenshot;
({ fileId: " file ", url: "not a url", userPrompt: " arbitrary " }) satisfies AppScreenshot;

// @ts-expect-error canonical branding requires every nullable field.
({ isDiscoverableApp: true }) satisfies AppBranding;
// @ts-expect-error discoverability is boolean.
({ category: null, developer: null, isDiscoverableApp: "true", privacyPolicy: null, termsOfService: null, website: null }) satisfies AppBranding;
// @ts-expect-error nullable branding values are strings or null.
({ category: 1, developer: null, isDiscoverableApp: true, privacyPolicy: null, termsOfService: null, website: null }) satisfies AppBranding;
// @ts-expect-error canonical branding is closed.
({ category: null, developer: null, isDiscoverableApp: true, privacyPolicy: null, termsOfService: null, website: null, future: true }) satisfies AppBranding;

// @ts-expect-error review status is required.
({}) satisfies AppReview;
// @ts-expect-error review status is a string.
({ status: null }) satisfies AppReview;
// @ts-expect-error canonical reviews are closed.
({ status: "ok", future: true }) satisfies AppReview;

// @ts-expect-error canonical screenshots require nullable fields.
({ userPrompt: "x" }) satisfies AppScreenshot;
// @ts-expect-error userPrompt is required.
({ fileId: null, url: null }) satisfies AppScreenshot;
// @ts-expect-error screenshot URLs are strings or null.
({ fileId: null, url: 1, userPrompt: "x" }) satisfies AppScreenshot;
// @ts-expect-error screenshot file ids are strings or null.
({ fileId: false, url: null, userPrompt: "x" }) satisfies AppScreenshot;
// @ts-expect-error canonical screenshots are closed.
({ fileId: null, url: null, userPrompt: "x", future: true }) satisfies AppScreenshot;

void (null as unknown as Contracts);
