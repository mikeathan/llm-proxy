// ProviderID is the canonical provider key set, derived from PROVIDER_IDS in
// constants/providers.ts (single source of truth; the backend mirrors this in
// models.ProviderIDs()).
import { PROVIDER_IDS } from '../constants/providers'

export type ProviderID = (typeof PROVIDER_IDS)[number]
