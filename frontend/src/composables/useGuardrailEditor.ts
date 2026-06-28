import { ref, watch, toRaw } from "vue"
import { listToString, stringToList } from "../utils/config"
import type { AgentGuardrailsConfig } from "../types/admin"
import type { Ref } from "vue"

function ensureStructure(cfg: any) {
  if (!cfg) cfg = {}
  if (!cfg.global)
    cfg.global = { block_secrets: true, user_blocked_patterns: [] }
  if (!cfg.terminal) {
    cfg.terminal = {
      enabled: false,
      allowed_commands: [],
      allowed_env_vars: [],
      path_extensions: [],
      timeout_seconds: 30,
      session_idle_timeout_seconds: 1800,
      max_output_size_chars: 5000,
    }
  }
  if (!cfg.filesystem)
    cfg.filesystem = {
      enabled: false,
      allowed_paths: [],
      read_only: true,
      max_file_size_kb: 512,
    }
  if (!cfg.search)
    cfg.search = { enabled: false, max_query_len: 100, blocked_sites: [] }
  if (!cfg.communication)
    cfg.communication = {
      enabled: false,
      require_review: true,
      max_messages_per_task: 10,
    }
  if (!cfg.network)
    cfg.network = {
      enabled: false,
      allow_lan_access: false,
      allow_internet_access: false,
      max_fetch_size_kb: 512,
      timeout_seconds: 30,
    }
  return cfg
}

export function useGuardrailEditor(modelValue: AgentGuardrailsConfig) {
  const local = ref<AgentGuardrailsConfig>(
    ensureStructure(structuredClone(toRaw(modelValue))),
  )

  watch(
    () => modelValue,
    (newVal) => {
      const current = JSON.stringify(toRaw(local.value))
      const incoming = JSON.stringify(toRaw(newVal))
      if (current !== incoming) {
        local.value = ensureStructure(structuredClone(toRaw(newVal)))
      }
    },
    { deep: true },
  )

  const terminalAllowedRaw = ref(listToString(local.value.terminal?.allowed_commands, "\n"))
  const terminalBlockedRaw = ref(listToString(local.value.terminal?.blocked_patterns, "\n"))
  const terminalEnvVarsRaw = ref(listToString(local.value.terminal?.allowed_env_vars, "\n"))
  const terminalPathExtensionsRaw = ref(listToString(local.value.terminal?.path_extensions, "\n"))
  const terminalExternalPathsRaw = ref(listToString(local.value.terminal?.allowed_external_paths, "\n"))
  const fsAllowedPathsRaw = ref(listToString(local.value.filesystem?.allowed_paths, "\n"))
  const fsAllowedExtensionsRaw = ref(listToString(local.value.filesystem?.allowed_extensions, "\n"))
  const fsBlockedFilenamesRaw = ref(listToString(local.value.filesystem?.blocked_filenames, "\n"))
  const searchBlockedSitesRaw = ref(listToString(local.value.search?.blocked_sites, "\n"))
  const networkBlockedDomainsRaw = ref(listToString(local.value.network?.blocked_domains, "\n"))
  const networkBlockedIPsRaw = ref(listToString(local.value.network?.blocked_ips, "\n"))
  const globalBlockedRaw = ref(listToString(local.value.global?.user_blocked_patterns, "\n"))

  function setupRawWatchers(emitUpdate: (val: AgentGuardrailsConfig) => void) {
    const syncRawFields = () => {
      const v = local.value
      syncRaw(terminalAllowedRaw, v.terminal?.allowed_commands)
      syncRaw(terminalBlockedRaw, v.terminal?.blocked_patterns)
      syncRaw(terminalEnvVarsRaw, v.terminal?.allowed_env_vars)
      syncRaw(terminalPathExtensionsRaw, v.terminal?.path_extensions)
      syncRaw(terminalExternalPathsRaw, v.terminal?.allowed_external_paths)
      syncRaw(fsAllowedPathsRaw, v.filesystem?.allowed_paths)
      syncRaw(fsAllowedExtensionsRaw, v.filesystem?.allowed_extensions)
      syncRaw(fsBlockedFilenamesRaw, v.filesystem?.blocked_filenames)
      syncRaw(searchBlockedSitesRaw, v.search?.blocked_sites)
      syncRaw(networkBlockedDomainsRaw, v.network?.blocked_domains)
      syncRaw(networkBlockedIPsRaw, v.network?.blocked_ips)
      syncRaw(globalBlockedRaw, v.global?.user_blocked_patterns)
    }

    watch(local, () => {
      syncRawFields()
      emitUpdate(structuredClone(toRaw(local.value)))
    }, { deep: true })
  }

  return {
    local,
    terminalAllowedRaw,
    terminalBlockedRaw,
    terminalEnvVarsRaw,
    terminalPathExtensionsRaw,
    terminalExternalPathsRaw,
    fsAllowedPathsRaw,
    fsAllowedExtensionsRaw,
    fsBlockedFilenamesRaw,
    searchBlockedSitesRaw,
    networkBlockedDomainsRaw,
    networkBlockedIPsRaw,
    globalBlockedRaw,
    setupRawWatchers,
  }
}

function syncRaw(rawRef: Ref<string>, list: string[] | undefined) {
  const clean = listToString(list, "\n")
  const current = listToString(stringToList(rawRef.value, "\n"), "\n")
  if (current !== clean) {
    rawRef.value = clean
  }
}
