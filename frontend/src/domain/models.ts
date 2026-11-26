import type { GlobalConfig } from "../types/admin";
import type { Model, ModelScope } from "../types/model";
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
