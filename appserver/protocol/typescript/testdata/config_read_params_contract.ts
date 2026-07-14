import type { ConfigReadParams } from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? (<T>() => T extends B ? 1 : 2) extends
        (<T>() => T extends A ? 1 : 2)
      ? true
      : false
    : false;
type Expect<T extends true> = T;

type ConfigReadParamsContract = Expect<Equal<ConfigReadParams, {
  cwd?: string | null;
  includeLayers?: boolean;
}>>;

({}) satisfies ConfigReadParams;
({ includeLayers: false }) satisfies ConfigReadParams;
({ includeLayers: true, cwd: null }) satisfies ConfigReadParams;
({ cwd: "" }) satisfies ConfigReadParams;
({ cwd: "relative/project" }) satisfies ConfigReadParams;
({ cwd: "/workspace/project" }) satisfies ConfigReadParams;

// @ts-expect-error includeLayers is a non-null boolean.
({ includeLayers: null }) satisfies ConfigReadParams;
// @ts-expect-error includeLayers is a boolean.
({ includeLayers: "false" }) satisfies ConfigReadParams;
// @ts-expect-error cwd is a nullable string.
({ cwd: false }) satisfies ConfigReadParams;
// @ts-expect-error optional cwd cannot be explicit undefined.
({ cwd: undefined }) satisfies ConfigReadParams;
// @ts-expect-error the public request excludes live keys.
({ keys: [] }) satisfies ConfigReadParams;
// @ts-expect-error the public request excludes live includeValues.
({ includeValues: true }) satisfies ConfigReadParams;
// @ts-expect-error wire fields use camel case.
({ include_layers: true }) satisfies ConfigReadParams;
// @ts-expect-error the request is closed.
({ extra: true }) satisfies ConfigReadParams;
// @ts-expect-error the request is an object.
null satisfies ConfigReadParams;

declare const configReadParamsContract: ConfigReadParamsContract;
void configReadParamsContract;
