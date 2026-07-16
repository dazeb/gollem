import type { AppSummary } from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type Contract = Expect<Equal<AppSummary, {
  id: string;
  name: string;
  description: string | null;
  installUrl: string | null;
  category: string | null;
}>>;

({
  id: "id",
  name: "name",
  description: null,
  installUrl: null,
  category: null,
}) satisfies AppSummary;

({
  id: " id ",
  name: "",
  description: " description ",
  installUrl: "not a url",
  category: " category ",
}) satisfies AppSummary;

// @ts-expect-error canonical AppSummary requires every field.
({ id: "id", name: "name" }) satisfies AppSummary;
// @ts-expect-error id is required.
({ name: "name", description: null, installUrl: null, category: null }) satisfies AppSummary;
// @ts-expect-error name is required.
({ id: "id", description: null, installUrl: null, category: null }) satisfies AppSummary;
// @ts-expect-error id is a string.
({ id: null, name: "name", description: null, installUrl: null, category: null }) satisfies AppSummary;
// @ts-expect-error name is a string.
({ id: "id", name: false, description: null, installUrl: null, category: null }) satisfies AppSummary;
// @ts-expect-error description is a string or null.
({ id: "id", name: "name", description: 1, installUrl: null, category: null }) satisfies AppSummary;
// @ts-expect-error installUrl is a string or null.
({ id: "id", name: "name", description: null, installUrl: [], category: null }) satisfies AppSummary;
// @ts-expect-error category is a string or null.
({ id: "id", name: "name", description: null, installUrl: null, category: {} }) satisfies AppSummary;
// @ts-expect-error canonical AppSummary is closed.
({ id: "id", name: "name", description: null, installUrl: null, category: null, future: true }) satisfies AppSummary;

void (null as unknown as Contract);
