import type { MarketplaceRemoveParams } from "../gollem_appserver_protocol";

({ marketplaceName: "official" }) satisfies MarketplaceRemoveParams;
// @ts-expect-error source requires marketplaceName.
({}) satisfies MarketplaceRemoveParams;
// @ts-expect-error marketplaceName is a string.
({ marketplaceName: 1 }) satisfies MarketplaceRemoveParams;
