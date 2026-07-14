import type {
  NetworkDomainPermission,
  NetworkRequirements,
  NetworkUnixSocketPermission,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? (<T>() => T extends B ? 1 : 2) extends
        (<T>() => T extends A ? 1 : 2)
      ? true
      : false
    : false;
type Expect<T extends true> = T;

type NetworkRequirementsContract = [
  Expect<Equal<NetworkDomainPermission, "allow" | "deny">>,
  Expect<Equal<NetworkUnixSocketPermission, "allow" | "deny">>,
  Expect<Equal<NetworkRequirements, {
    enabled: boolean | null;
    httpPort: number | null;
    socksPort: number | null;
    allowUpstreamProxy: boolean | null;
    dangerouslyAllowNonLoopbackProxy: boolean | null;
    dangerouslyAllowAllUnixSockets: boolean | null;
    domains: { [key in string]?: NetworkDomainPermission } | null;
    managedAllowedDomainsOnly: boolean | null;
    allowedDomains: string[] | null;
    deniedDomains: string[] | null;
    unixSockets: { [key in string]?: NetworkUnixSocketPermission } | null;
    allowUnixSockets: string[] | null;
    allowLocalBinding: boolean | null;
  }>>,
];

"allow" satisfies NetworkDomainPermission;
"deny" satisfies NetworkDomainPermission;
"allow" satisfies NetworkUnixSocketPermission;
"deny" satisfies NetworkUnixSocketPermission;

export const nullNetworkRequirements = {
  enabled: null,
  httpPort: null,
  socksPort: null,
  allowUpstreamProxy: null,
  dangerouslyAllowNonLoopbackProxy: null,
  dangerouslyAllowAllUnixSockets: null,
  domains: null,
  managedAllowedDomainsOnly: null,
  allowedDomains: null,
  deniedDomains: null,
  unixSockets: null,
  allowUnixSockets: null,
  allowLocalBinding: null,
} satisfies NetworkRequirements;

export const fullNetworkRequirements = {
  enabled: false,
  httpPort: 0,
  socksPort: 65535,
  allowUpstreamProxy: true,
  dangerouslyAllowNonLoopbackProxy: false,
  dangerouslyAllowAllUnixSockets: true,
  domains: { "": "allow", "example.com": "deny" },
  managedAllowedDomainsOnly: false,
  allowedDomains: ["", "example.com", "example.com"],
  deniedDomains: [],
  unixSockets: { "": "deny", "/tmp/codex.sock": "allow" },
  allowUnixSockets: ["", "/tmp/codex.sock", "/tmp/codex.sock"],
  allowLocalBinding: true,
} satisfies NetworkRequirements;

// @ts-expect-error domain permissions are closed.
"permit" satisfies NetworkDomainPermission;
// @ts-expect-error Unix-socket permissions are closed.
"permit" satisfies NetworkUnixSocketPermission;
// @ts-expect-error every network-requirements field is required.
({}) satisfies NetworkRequirements;
// @ts-expect-error enabled is required nullable.
({ enabled: null }) satisfies NetworkRequirements;
// @ts-expect-error socksPort is required nullable.
({ ...nullNetworkRequirements, socksPort: undefined }) satisfies NetworkRequirements;
// @ts-expect-error ports are nullable numbers.
({ ...nullNetworkRequirements, httpPort: "80" }) satisfies NetworkRequirements;
// @ts-expect-error domain maps exclude null values.
({ ...nullNetworkRequirements, domains: { "example.com": null } }) satisfies NetworkRequirements;
// @ts-expect-error domain map values are closed.
({ ...nullNetworkRequirements, domains: { "example.com": "permit" } }) satisfies NetworkRequirements;
// @ts-expect-error allowed-domain arrays exclude null members.
({ ...nullNetworkRequirements, allowedDomains: [null] }) satisfies NetworkRequirements;
// @ts-expect-error denied-domain arrays contain strings.
({ ...nullNetworkRequirements, deniedDomains: [1] }) satisfies NetworkRequirements;
// @ts-expect-error Unix-socket maps exclude null values.
({ ...nullNetworkRequirements, unixSockets: { "/tmp/codex.sock": null } }) satisfies NetworkRequirements;
// @ts-expect-error Unix-socket map values are closed.
({ ...nullNetworkRequirements, unixSockets: { "/tmp/codex.sock": "permit" } }) satisfies NetworkRequirements;
// @ts-expect-error allowed Unix-socket arrays exclude null members.
({ ...nullNetworkRequirements, allowUnixSockets: [null] }) satisfies NetworkRequirements;
// @ts-expect-error allowLocalBinding is a nullable boolean.
({ ...nullNetworkRequirements, allowLocalBinding: 1 }) satisfies NetworkRequirements;
// @ts-expect-error network requirements are closed.
({ ...nullNetworkRequirements, extra: true }) satisfies NetworkRequirements;

declare const contract: NetworkRequirementsContract;
void contract;
