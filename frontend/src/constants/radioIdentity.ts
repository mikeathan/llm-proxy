import type { RadioDeviceKind, RadioKindSource, RadioOwnership } from '../types/radio'

// The words of Radio's device identity (ADR-0058): what a device is, who made it, whose it probably is and why. A Go test holds the kind sources
// and the feature words here to the server's lists, so none shows as a raw id.

export const RADIO_KIND_WORDS: Record<RadioDeviceKind, string> = {
  router: 'Router',
  phone: 'Phone or tablet',
  computer: 'Computer',
  tv: 'TV or streamer',
  speaker: 'Speaker',
  smart_home: 'Smart-home device',
  camera: 'Camera or doorbell',
  wearable: 'Watch or wearable',
  printer: 'Printer',
  console: 'Games console',
  unknown: 'Device',
}

/** The kinds as the owner picks one, in the server's order. */
export const RADIO_KIND_ORDER: RadioDeviceKind[] = ['router', 'phone', 'computer', 'tv', 'speaker', 'smart_home', 'camera', 'wearable', 'printer', 'console', 'unknown']

/** The icon for each kind (assets/svg). */
export const RADIO_KIND_ICON: Record<RadioDeviceKind, string> = {
  router: 'router', phone: 'phone', computer: 'computer', tv: 'tv', speaker: 'speaker', smart_home: 'smart-home', camera: 'camera',
  wearable: 'wearable', printer: 'printer', console: 'console', unknown: 'device',
}

export const RADIO_KIND_SOURCES: Record<RadioKindSource, string> = {
  owner_said: 'you said so',
  announced_type: 'the device announces its type',
  model_name: 'from its model name',
  learned_from_you: 'learned from what you said about other devices of this maker',
  sends_beacons: 'it broadcasts a Wi-Fi network',
  maker: 'from its maker',
  vendor_element: 'a hint in what it sends',
  private_address: 'it uses a private address, as phones, tablets and laptops do',
  no_evidence: 'nothing it sends says what it is',
}

/** The numbers the learned classifier weighs, as the device page names them. */
export const RADIO_FACTOR_WORDS: Record<string, string> = {
  share: 'how often it is heard',
  signal_mean: 'how strong it usually is',
  signal_spread: 'how steady its signal is',
  signal_max: 'its strongest signal',
  days: 'how many days it has been here',
  night: 'whether it is heard at night',
  movement: 'how much it moves',
  access_point: 'whether it broadcasts a network',
  private_address: 'whether it uses a private address',
  uses_your_wifi: 'whether it uses your Wi-Fi',
  fixed_kind: 'whether it is a kind of thing that stays put',
}

export const RADIO_PRIVATE_ADDRESS = 'private address'
export const RADIO_UNKNOWN_DEVICE = 'Unknown device'
export const RADIO_HINT_SUFFIX = '(hint)'
export const radioBrandKind = (brand: string, kind: string): string => `${brand} ${kind.toLowerCase()}`
export const radioKindPrivate = (kind: string): string => `${kind} (${RADIO_PRIVATE_ADDRESS})`

/** How sure, in a word. */
export const radioSureWord = (confidence: number): string => (confidence >= 80 ? 'Very likely' : confidence >= 60 ? 'Probably' : 'Possibly')

// Whose it is: the one-line conclusion a row and the device page lead with.
export const RADIO_OWN_MINE = 'Yours'
export const RADIO_OWN_YOUR_ROUTER = 'Your router'
export const RADIO_OWN_ROUTER_RADIO = 'A radio of your router'
export const radioOwnOnLAN = (name: string): string => `On your network as “${name}”`
export const RADIO_OWN_ON_WIFI = 'Uses your Wi-Fi'
export const RADIO_OWN_NEIGHBOUR_ROUTER = 'A neighbour’s router'
export const RADIO_OWN_NEIGHBOUR = 'A neighbour’s device'
export const RADIO_OWN_PASSING = 'Passing by'
export const RADIO_OWN_HOUSE = 'Always here'
export const RADIO_OWN_PERSONAL = 'Comes and goes'
export const RADIO_OWN_UNSURE = 'Not sure yet'
export const RADIO_FIRST_LOOK: Record<'mobile' | 'neighbour', string> = {
  mobile: 'First look: probably someone’s phone, watch or earbuds',
  neighbour: 'First look: probably a neighbour’s',
}
export const radioEvidenceLine = (e: { share_percent: number; days: number; night_days: number; move_percent: number }): string =>
  `Heard in ${e.share_percent}% of readings over ${e.days} ${e.days === 1 ? 'day' : 'days'}` +
  (e.night_days ? `, at night on ${e.night_days} of them` : ', never at night') +
  (e.move_percent ? `; moved in ${e.move_percent}% of readings.` : '; never moved.')
export const RADIO_DEVICE_EVIDENCE = 'What it does'
export const radioBeyondLine = (n: number): string => `${n} beyond the edge`
export const radioBriefHidden = (n: number): string => `${n} ${n === 1 ? 'passer-by' : 'passers-by'} hidden`

// The ownership badge on a row.
export const RADIO_OWNERSHIP_WORDS: Record<RadioOwnership, string> = { mine: 'Yours', likely: 'Likely yours', neighbour: 'Not yours', unknown: 'Unknown' }

// The extra filter chips.
export const RADIO_FILTER_LIKELY = 'Likely yours'
export const RADIO_FILTER_ROUTERS = 'Routers'
export const RADIO_FILTER_NEW = 'New today'
export const RADIO_FILTER_HIDE_BRIEF = 'Hide passers-by'

// The networks view.
export const RADIO_NETWORKS_LOADING = 'Reading the networks…'
export const RADIO_NETWORKS_NONE = 'No router heard yet.'
export const RADIO_NETWORKS_YOURS = 'Your router'
export const RADIO_NETWORKS_OTHER = 'Not yours'
export const radioRadiosLine = (n: number): string => `${n} ${n === 1 ? 'radio' : 'radios'}`
export const radioClientsLine = (n: number): string => (n === 0 ? 'no device seen using it' : `${n} ${n === 1 ? 'device uses it' : 'devices use it'}`)
export const RADIO_NETWORKS_UNCONNECTED = 'Not connected to any network heard'
export const RADIO_NETWORKS_UNCONNECTED_HELP = 'Devices only looking for networks, or using one out of range.'
export const RADIO_NETWORK_HIDDEN = 'hidden network'

// The "help it learn" card.
export const RADIO_ASK_TITLE = 'Help it learn'
export const RADIO_ASK_INTRO = 'Your answers train the sorting for this house. A few at a time, the ones it is least sure of.'
export const RADIO_ASK_REASONS: Record<string, string> = {
  likely_yours: 'The evidence says this may be yours.',
  close_and_unsure: 'Close by, and the sorting is not sure.',
}
export const RADIO_ASK_MINE = 'Mine'
export const RADIO_ASK_NOT_MINE = 'Not mine'
export const radioLearnedLine = (owner: number, agree: number, checked: number): string =>
  owner === 0 ? 'No answers yet: sorting from the starting rules and what it hears.' : checked > 0 ? `Learned from ${owner} of your answers; it gives back ${agree} of ${checked} when one is held out.` : `Learned from ${owner} of your answers.`
export const RADIO_ASK_FAILED = 'The answer could not be saved.'

// The device page.
export const RADIO_DEVICE_BACK = 'Around you'
export const RADIO_DEVICE_LOADING = 'Reading the device…'
export const RADIO_DEVICE_GONE = 'This device is not in the picture any more: it has not been heard in the last few days.'
export const RADIO_DEVICE_WHAT = 'What it is'
export const RADIO_DEVICE_WHOSE = 'Whose it is'
export const RADIO_DEVICE_WHERE = 'Where it is'
export const RADIO_DEVICE_NETWORK = 'Network'
export const RADIO_DEVICE_HISTORY = 'How its sorting changed'
export const RADIO_DEVICE_ANSWER = 'Your answer'
export const RADIO_DEVICE_KIND_LABEL = 'It is a'
export const RADIO_DEVICE_MAKER = 'Maker'
export const RADIO_DEVICE_MODEL = 'Model'
export const RADIO_DEVICE_ADDRESS = 'Address'
export const RADIO_DEVICE_ADDRESS_FIXED = 'fixed (its maker can be read)'
export const RADIO_DEVICE_ADDRESS_PRIVATE = 'private (made up by the device)'
export const RADIO_DEVICE_WIFI = 'Wi-Fi'
export const RADIO_DEVICE_SHORT_ID = 'Short id'
export const RADIO_DEVICE_CHANCES = 'Sorting'
export const RADIO_DEVICE_FACTORS = 'Decided mostly by'
export const RADIO_DEVICE_USES = 'Uses'
export const RADIO_DEVICE_RADIOS = 'Other radios of the same router'
export const RADIO_DEVICE_CLIENTS = 'Devices using it'
export const RADIO_DEVICE_FAMILY = (n: number): string => `${n} other private ${n === 1 ? 'address sends' : 'addresses send'} the same signature: the same kind of device, possibly the same one.`
export const RADIO_DEVICE_NOT_MINE = 'Not mine'
export const RADIO_DEVICE_HAND_BACK = 'Let it decide'
export const RADIO_DEVICE_CALIBRATE_AT = 'Distance from this computer (m)'
export const RADIO_DEVICE_CALIBRATE = 'Calibrate the radar with it'
export const RADIO_DEVICE_CALIBRATE_HELP = 'Measure how far this device of yours is from the computer: every distance on the radar is then worked out from it.'
export const RADIO_DEVICE_SAVED = 'Saved: the sorting uses it from the next reading.'
export const RADIO_MOTION_WORDS: Record<'still' | 'moving', string> = { still: 'staying put', moving: 'moving about' }
export const radioOnWiFiCycles = (n: number): string => `Seen using your Wi-Fi in ${n} ${n === 1 ? 'reading' : 'readings'}.`
export const RADIO_EVIDENCE_LAN = 'Your network’s device list has its address.'
export const RADIO_EVIDENCE_ROUTER = 'Its address sits next to your router’s: routers number their Wi-Fi radios from their own address.'
export const RADIO_EVIDENCE_YOUR_AP = 'It broadcasts one of your network names and you confirmed it.'
export const RADIO_EVIDENCE_BOX_YOURS = 'It is a radio of the same router as one of your access points.'
export const RADIO_EVIDENCE_NET_YOURS = 'It uses one of your access points.'
export const RADIO_EVIDENCE_NET_OTHER = 'It uses a network that is not yours.'
export const RADIO_EVIDENCE_NONE = 'Nothing it sends links it to your network yet.'

// The radar's zoom.
export const RADIO_ZOOM_IN = 'Zoom in'
export const RADIO_ZOOM_OUT = 'Zoom out'
export const RADIO_ZOOM_RESET = 'Show all'
export const radioZoomLine = (m: number): string => `Edge: ${m} m`
export const RADIO_RADAR_BEYOND = 'beyond the edge'
export const RADIO_RADAR_SOLO_HELP = 'Only its distance is measured, not which way: it could be anywhere on the shaded ring.'

// The presence strip's legend.
export const RADIO_PRESENCE_LEGEND_HEARD = 'Heard'
export const RADIO_PRESENCE_LEGEND_AWAY = 'Listened, not heard'
export const RADIO_PRESENCE_LEGEND_OFF = 'Not listening'
export const RADIO_DETAIL_OPEN = 'Open page'
export const RADIO_DEVICE_GONE_TITLE = 'Not heard lately'
export const RADIO_DEVICE_FAILED_TITLE = 'Could not read'
/** One of a device's own class changes: "neighbour → personal, you said so". */
export const radioDeviceChange = (from: string, to: string, reason: string, classWords: Record<string, string>, reasons: Record<string, string>): string =>
  `${from ? `${classWords[from] ?? from} → ` : ''}${classWords[to] ?? to}, ${reasons[reason] ?? reason}`
export const RADIO_AP_GUEST = 'Guest network'
export const RADIO_AP_GUEST_HELP = 'Visitors use it: a device on it is not counted as yours.'
export const radioBroadcasts = (name: string): string => `broadcasts “${name}”`
export const RADIO_RADAR_USUAL_SHOW_BRIEF = 'Show them'
export const RADIO_MY_DEVICE = 'My device'
