import type { SearchProvider } from "../types/admin";

// Canonical search-provider key set — a typing aid / offline fallback only. The
// Settings dropdown is driven by the backend list (config.search_providers); the
// backend mirrors this set in models.SearchProviderIDs(). Keep the two in sync.
export const SEARCH_PROVIDER_IDS: readonly SearchProvider[] = ["tavily", "brave", "serpapi"];

export const SEARCH_PROVIDER_LABELS: Record<SearchProvider, string> = {
  tavily: "Tavily",
  brave: "Brave Search",
  serpapi: "SerpAPI",
};

// Mirrors backend models.DefaultSearchMaxResults / MaxSearchMaxResults. The
// backend re-validates on save, so these bound the input only.
export const SEARCH_MAX_RESULTS_MIN = 1;
export const SEARCH_MAX_RESULTS_MAX = 20;
export const SEARCH_MAX_RESULTS_DEFAULT = 5;

// Resolves a provider id to its display label, falling back to the raw id for a
// value the frontend does not know (backend-driven, forward-compatible).
export const searchProviderLabel = (id: string): string =>
  SEARCH_PROVIDER_LABELS[id as SearchProvider] ?? id;
