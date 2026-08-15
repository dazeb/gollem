import type { MarketplaceUpgradeParams } from "../gollem_appserver_protocol";

({}) satisfies MarketplaceUpgradeParams;
({ marketplaceName: "official" }) satisfies MarketplaceUpgradeParams;
({ marketplaceName: null }) satisfies MarketplaceUpgradeParams;
// @ts-expect-error marketplaceName is a string when supplied.
({ marketplaceName: 1 }) satisfies MarketplaceUpgradeParams;
