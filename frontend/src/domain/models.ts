import type { GlobalConfig, ProviderType } from "../types/admin";
import type { KeyModels, Model, ModelScope, ProviderRailEntry } from "../types/model";
import type { StatusState } from "../types/ui";

// The Models page's catalogue rules: which models a filter shows, in what order, and the state and role of each.

const LOCAL = "local";

export const isLocalModel = (m: Model): boolean => m.provider === LOCAL;

/** A model's state as a tag: a local model is loading, running or idle; a cloud model is called on demand. */
export function modelStatus(m: Model): { state: StatusState; label: string } {
  if (m.active && !m.ready) return { state: "running", label: "Loading" };
  if (m.active) return { state: "success", label: "Running" };
  return isLocalModel(m) ? { state: "neutral", label: "Idle" } : { state: "info", label: "Cloud" };
}

/** What routing gives a model: the primary answers requests that name no model, the fallback takes over when it fails. */
export function routingRole(m: Model, config: Pick<GlobalConfig, "primary_model" | "fallback_model"> | undefined): "Primary" | "Fallback" | null {
  if (config?.primary_model === m.name) return "Primary";
  if (config?.fallback_model === m.name) return "Fallback";
  return null;
}

/** The running model first, then the other local models, then the cloud ones; the order within each is kept. */
export function orderModels(models: readonly Model[]): Model[] {
  const rank = (m: Model) => (m.active ? 0 : isLocalModel(m) ? 1 : 2);
  return [...models].map((m, i) => ({ m, i })).sort((a, b) => rank(a.m) - rank(b.m) || a.i - b.i).map(({ m }) => m);
}

/** The models a scope and a typed filter show (the filter matches the name, the file or the model id, without case). */
export function filterModels(models: readonly Model[], scope: ModelScope, text: string): Model[] {
  const q = text.trim().toLowerCase();
  return models.filter((m) => {
    if (scope === "local" && !isLocalModel(m)) return false;
    if (scope === "cloud" && isLocalModel(m)) return false;
    if (!q) return true;
    return [m.name, m.filename, m.model_id].some((v) => !!v && v.toLowerCase().includes(q));
  });
}

/**
 * A model in a picker: its name, then where it runs ("deepseek-v4.1-flash · OpenCode Go"), so the same model on two providers
 * reads as two choices. Names are unique in the registry, so the value stays the name; only the label carries the provider.
 */
export const modelChoiceLabel = (m: Pick<Model, "name" | "provider">, providerLabel: (p: ProviderType) => string): string => `${m.name} · ${modelProviderName(m, providerLabel)}`;

/** Where a model runs, as a person reads it: "Local", or the provider's display name. */
export const modelProviderName = (m: Pick<Model, "provider">, providerLabel: (p: ProviderType) => string): string => (m.provider === LOCAL ? "Local" : providerLabel(m.provider));

const plural = (n: number, one: string) => `${n} ${one}${n === 1 ? "" : "s"}`;

/** What a cloud provider holds, in a few words: not set up until it has a key. */
export const providerSummary = (keys: number, models: number): string => (keys ? `${plural(keys, "key")} · ${plural(models, "model")}` : "Not set up");

/**
 * A provider's models grouped under the key that serves each (provider_config.api_key_name): one group per key in the keys' order, even
 * one with no model yet, then one group with an empty key name for models whose key is gone or unnamed.
 */
export function groupModelsByKey(models: readonly Model[], keyNames: readonly string[]): KeyModels[] {
  const groups: KeyModels[] = keyNames.map((key) => ({ key, models: models.filter((m) => m.provider_config?.api_key_name === key) }));
  const orphans = models.filter((m) => !keyNames.includes(m.provider_config?.api_key_name ?? ""));
  return orphans.length ? [...groups, { key: "", models: orphans }] : groups;
}

/** The provider rail: All models, This computer, then each cloud provider with what it holds; error when its last key test failed. */
export function railEntries(providers: readonly { id: ProviderType; label: string }[], models: readonly Model[], keyCount: (p: ProviderType) => number, keyFailed: (p: ProviderType) => boolean = () => false): ProviderRailEntry[] {
  const local = models.filter(isLocalModel);
  return [
    { id: "all", label: "All models", summary: plural(models.length, "model"), state: "neutral" },
    { id: LOCAL, label: "This computer", summary: plural(local.length, "model"), state: local.some((m) => m.active) ? "success" : "neutral" },
    ...providers.map((p) => {
      const mine = models.filter((m) => m.provider === p.id).length;
      const broken = keyCount(p.id) > 0 && keyFailed(p.id);
      const summary = providerSummary(keyCount(p.id), mine) + (broken ? " · key test failed" : "");
      return { id: p.id, label: p.label, summary, state: broken ? ("error" as const) : keyCount(p.id) ? ("info" as const) : ("neutral" as const) };
    }),
  ];
}
