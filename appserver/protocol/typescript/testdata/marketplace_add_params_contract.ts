import type { MarketplaceAddParams } from "../gollem_appserver_protocol";

({ source: "https://example.com/repo" }) satisfies MarketplaceAddParams;
({ source: "repo", refName: null, sparsePaths: ["plugins"] }) satisfies MarketplaceAddParams;
// @ts-expect-error source is required.
({ refName: "main" }) satisfies MarketplaceAddParams;
// @ts-expect-error sparse paths contain strings.
({ source: "repo", sparsePaths: [1] }) satisfies MarketplaceAddParams;
