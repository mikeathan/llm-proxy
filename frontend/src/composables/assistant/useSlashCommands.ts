import { computed, onUnmounted, ref, watch, type Ref } from 'vue'
import { CommandService } from '../../services/commands/commandService'
import type { CommandInfo, SlashPreview } from '../../types/commands'

// A command is "/name" at the start; the menu shows while only the name is typed, the helper after the space.
const COMMAND_START = /^\/([A-Za-z]*)$/
const COMMAND_WITH_ARGS = /^\/([A-Za-z]+)\s/
export const PREVIEW_DEBOUNCE_MS = 250

// Slash-command help for the chat box: the menu of commands, the active command's usage and examples, and a live
// preview of what the typed text would do (answered by the backend, so it can never disagree with what runs).
export function useSlashCommands(text: Ref<string>, workspaceId?: Ref<string | undefined>) {
  const commands = ref<CommandInfo[]>([])
  const preview = ref<SlashPreview | null>(null)
  let loaded = false
  let timer: ReturnType<typeof setTimeout> | undefined
  let latest = 0

  async function loadCommands() {
    if (loaded) return
    loaded = true
    try {
      commands.value = await CommandService.list()
    } catch {
      loaded = false
    }
  }

  const suggestions = computed(() => {
    const m = COMMAND_START.exec(text.value)
    if (!m) return []
    const typed = (m[1] ?? '').toLowerCase()
    return commands.value.filter(c => c.name.startsWith(typed))
  })

  const active = computed(() => {
    const m = COMMAND_WITH_ARGS.exec(text.value)
    const name = (m?.[1] ?? '').toLowerCase()
    return m ? commands.value.find(c => c.name === name) ?? null : null
  })

  async function refreshPreview(message: string) {
    const mine = ++latest
    try {
      const p = await CommandService.preview(message, undefined, workspaceId?.value)
      if (mine !== latest) return
      preview.value = p.matched ? { text: p.text ?? '', error: p.error ?? '' } : null
    } catch {
      if (mine === latest) preview.value = null
    }
  }

  // Also re-runs when the command list arrives after the text was typed.
  watch([text, active], ([value]) => {
    clearTimeout(timer)
    if (value.startsWith('/')) void loadCommands()
    if (!active.value || !value.trim()) {
      latest++
      preview.value = null
      return
    }
    timer = setTimeout(() => void refreshPreview(value), PREVIEW_DEBOUNCE_MS)
  })

  onUnmounted(() => clearTimeout(timer))

  // "/remind " ready for the arguments.
  const completion = (command: CommandInfo): string => `/${command.name} `
  // An example filled in after its command, replacing whatever arguments were typed.
  const withExample = (command: CommandInfo, example: string): string => `/${command.name} ${example}`

  return { commands, suggestions, active, preview, completion, withExample }
}
