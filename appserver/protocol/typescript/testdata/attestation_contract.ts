import type {
  AttestationGenerateParams,
  AttestationGenerateResponse,
  MethodParamsByName,
  MethodResultsByName,
} from "../gollem_appserver_protocol.js";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type Contracts = [
  Expect<Equal<AttestationGenerateParams, Record<string, never>>>,
  Expect<Equal<AttestationGenerateResponse, { token: string }>>,
  Expect<Equal<Extract<keyof MethodParamsByName, "attestation/generate">, never>>,
  Expect<Equal<Extract<keyof MethodResultsByName, "attestation/generate">, never>>,
];

({}) satisfies AttestationGenerateParams;
({ token: "" }) satisfies AttestationGenerateResponse;
({ token: "opaque token" }) satisfies AttestationGenerateResponse;

// @ts-expect-error empty params have no known fields.
({ future: true }) satisfies AttestationGenerateParams;
// @ts-expect-error params are non-null objects.
(null) satisfies AttestationGenerateParams;
// @ts-expect-error params are not arrays.
([]) satisfies AttestationGenerateParams;
// @ts-expect-error params are not strings.
("") satisfies AttestationGenerateParams;
// @ts-expect-error params are not numbers.
(0) satisfies AttestationGenerateParams;
// @ts-expect-error params are not booleans.
(false) satisfies AttestationGenerateParams;

// @ts-expect-error token is required.
({}) satisfies AttestationGenerateResponse;
// @ts-expect-error token is non-null.
({ token: null }) satisfies AttestationGenerateResponse;
// @ts-expect-error token is a string.
({ token: 1 }) satisfies AttestationGenerateResponse;
// @ts-expect-error responses have no extra fields.
({ token: "value", extra: true }) satisfies AttestationGenerateResponse;
// @ts-expect-error responses are non-null objects.
(null) satisfies AttestationGenerateResponse;

void (null as unknown as Contracts);
