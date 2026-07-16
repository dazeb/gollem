import type {
  CommandExecutionApprovalRequestParams,
  MethodParamsByName,
  MethodResultsByName,
  NetworkApprovalContext,
  NetworkApprovalProtocol,
} from "../gollem_appserver_protocol.js";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
    (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type ExactContext = {
  host: string;
  protocol: NetworkApprovalProtocol;
};

type Contracts = [
  Expect<Equal<NetworkApprovalContext, ExactContext>>,
  Expect<Equal<NetworkApprovalProtocol, "http" | "https" | "socks5Tcp" | "socks5Udp">>,
  Expect<Equal<Extract<keyof CommandExecutionApprovalRequestParams, "networkApprovalContext">, never>>,
  Expect<Equal<Extract<MethodParamsByName[keyof MethodParamsByName], NetworkApprovalContext>, never>>,
  Expect<Equal<Extract<MethodResultsByName[keyof MethodResultsByName], NetworkApprovalContext>, never>>,
];

({ host: "", protocol: "http" }) satisfies NetworkApprovalContext;
({ host: "example.com:443", protocol: "https" }) satisfies NetworkApprovalContext;
({ host: "arbitrary host", protocol: "socks5Udp" }) satisfies NetworkApprovalContext;

// @ts-expect-error host is required.
({ protocol: "http" }) satisfies NetworkApprovalContext;
// @ts-expect-error protocol is required.
({ host: "host" }) satisfies NetworkApprovalContext;
// @ts-expect-error host is a non-null string.
({ host: null, protocol: "http" }) satisfies NetworkApprovalContext;
// @ts-expect-error protocol is closed.
({ host: "host", protocol: "ftp" }) satisfies NetworkApprovalContext;
// @ts-expect-error canonical records are closed.
({ host: "host", protocol: "http", future: true }) satisfies NetworkApprovalContext;
// @ts-expect-error values are non-null objects.
(null) satisfies NetworkApprovalContext;
// @ts-expect-error values are not arrays.
([]) satisfies NetworkApprovalContext;

void (null as unknown as Contracts);
