import { computed, ref, type Ref } from "vue"
import { useRouter } from "vue-router"
import { usePersistedState } from "../ui/usePersistedState"
import { RADIO_VIEW_ORDER } from "../../constants/radio"
import { byCloseness, narrow } from "../../utils/radio/around"
import { toRadioDevice } from "../../router/routes"
import type { RadioAroundFilter, RadioAroundState, RadioAroundToggles, RadioAroundView, RadioLive, RadioLiveDevice } from "../../types/radio"

// Radio · Around you: what the owner has chosen to look at. One state, shared by the radar and the list of whichever view is open, so a chip or a
// channel narrows both. A tap on a device (a dot, a row, a change) opens its own page. The view and the toggles are remembered per person.
const KEY_VIEW = "radio-around-view"
const KEY_TOGGLES = "radio-around-toggles"
const VERSION = 2

// Passers-by are hidden by default (after nodeherder's "tracked, not census"): the list answers "what is around the house", and the count of what is
// hidden is always said, so the picture never silently leaves something out.
const DEFAULT_TOGGLES: RadioAroundToggles = { range: false, spoke: false, colours: false, usual: false, brief: true }

const parseView = (data: unknown): RadioAroundView | null => (RADIO_VIEW_ORDER.includes(data as RadioAroundView) ? (data as RadioAroundView) : null)
const parseToggles = (data: unknown): RadioAroundToggles | null => {
  if (typeof data !== "object" || data === null) return null
  const d = data as Record<string, unknown>
  return { ...DEFAULT_TOGGLES, ...Object.fromEntries(Object.keys(DEFAULT_TOGGLES).filter((k) => typeof d[k] === "boolean").map((k) => [k, d[k]])) }
}

export function useRadioAround(view: Ref<RadioLive | null>): RadioAroundState {
  const mode = usePersistedState<RadioAroundView>(KEY_VIEW, { version: VERSION, parse: parseView, fallback: "radar" })
  const toggles = usePersistedState<RadioAroundToggles>(KEY_TOGGLES, { version: VERSION, parse: parseToggles, fallback: DEFAULT_TOGGLES })
  const filter = ref<RadioAroundFilter>("all")
  const channel = ref<number | null>(null)
  const today = ref(new Date().toISOString().slice(0, 10))

  const everyone = computed<RadioLiveDevice[]>(() => view.value?.devices ?? [])
  const shown = computed(() => {
    const list = byCloseness(narrow(everyone.value, { filter: filter.value, channel: channel.value, showUsual: toggles.value.usual, hideBrief: toggles.value.brief, today: today.value }))
    return [...list.filter((d) => !d.usual), ...list.filter((d) => d.usual)] // heard now first, then the ones usually here
  })
  /** Devices that match the chips and the channel but are usually here and not heard now, which the "usual" switch keeps out of the radar and the list. */
  const usualHidden = computed(() => (toggles.value.usual ? 0 : narrow(everyone.value, { filter: filter.value, channel: channel.value, showUsual: true, hideBrief: toggles.value.brief, today: today.value }).filter((d) => d.usual).length))
  /** How many passers-by the switch keeps out of the radar and the list. */
  const briefHidden = computed(() => (toggles.value.brief ? narrow(everyone.value, { filter: filter.value, channel: channel.value, showUsual: toggles.value.usual, today: today.value }).filter((d) => !d.mine && d.class === "transient").length : 0))
  const router = useRouter()

  /** Opens a device's own page: what it is, whose it is and why, where it is, and the owner's answers. */
  function open(id: string): void { void router.push(toRadioDevice(id)) }
  function setToggle(key: keyof RadioAroundToggles, on: boolean): void { toggles.value = { ...toggles.value, [key]: on } }
  function setFilter(next: RadioAroundFilter): void { filter.value = next }
  function setChannel(next: number | null): void { channel.value = channel.value === next ? null : next }

  return { mode, toggles, filter, channel, everyone, shown, usualHidden, briefHidden, open, setToggle, setFilter, setChannel, today }
}
