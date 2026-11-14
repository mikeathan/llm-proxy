import type { CapabilityUse, RadioClass, RadioHourState, RadioKind, RadioRing, RadioRole, RadioSignal, RadioTier, RadioTrend } from '../types/radio'
import type { StatusState } from '../types/ui'

// The words of the Radio · Sensors page. Capability titles and notes come from the server; these are the fixed phrases around them.

export const RADIO_SENSORS_TITLE = 'Sensors'
export const RADIO_SENSORS_INTRO = 'The radios on this computer and what each is for. Looking at this page sends nothing and listens to nothing.'
export const RADIO_OFF_TITLE = 'The Radio stack is off'
export const RADIO_OFF_BODY = 'Nothing listens until you switch it on in Modules.'
export const RADIO_OFF_ACTION = 'Open Modules'
export const RADIO_EMPTY_TITLE = 'No radio to listen with'
export const RADIO_EMPTY_BODY = 'Plug in a USB Wi-Fi adapter and look again. With no adapter the stack does nothing at all.'
export const RADIO_RELOAD = 'Look again'
export const RADIO_LOAD_FAILED_TITLE = 'The radios could not be read'
export const RADIO_LOAD_NEXT = 'Check that the server is running, then retry.'

export const RADIO_KIND_WORDS: Record<RadioKind, string> = {
  wifi: 'Wi-Fi',
  bluetooth: 'Bluetooth',
  ieee802154: 'Zigbee / Thread',
}

/** The one word a sensor leads with, and the state colour that goes with it. */
export const RADIO_ROLE_WORDS: Record<RadioRole, string> = {
  listening: 'Listening now',
  can_listen: 'Can listen',
  wont_use: 'Will not be used',
  no_driver: 'No driver yet',
}
export const RADIO_ROLE_STATE: Record<RadioRole, StatusState> = {
  listening: 'running',
  can_listen: 'success',
  wont_use: 'neutral',
  no_driver: 'queued',
}

export const RADIO_USE_WORDS: Record<CapabilityUse, string> = {
  used: 'In use',
  ready: 'Ready',
  unsupported: 'Not supported',
  unused_by_design: 'Refused, by design',
  unused_not_built: 'Not built yet',
  found_no_driver: 'No driver yet',
}
export const RADIO_USE_STATE: Record<CapabilityUse, StatusState> = {
  used: 'running',
  ready: 'success',
  unsupported: 'neutral',
  unused_by_design: 'neutral',
  unused_not_built: 'queued',
  found_no_driver: 'queued',
}

export const RADIO_CAPABILITIES_TITLE = 'Details'
export const RADIO_NOT_READ_YET = 'not read from the adapter yet'
export const RADIO_PROBE_PENDING = 'What this adapter supports is read from it the first time you listen.'
export const radioDetailsSummary = (n: number): string => `${n} things it can do`
export const radioProbeLine = (bands: string[], standards: string[], monitor: boolean): string =>
  [bands.join(' and '), standards.join(', '), monitor ? 'monitor mode supported' : 'no monitor mode'].filter(Boolean).join(' · ')
export const radioFoundLine = (listen: number, found: number): string =>
  `${found} ${found === 1 ? 'radio' : 'radios'} found, ${listen === 0 ? 'none can' : `${listen} can`} listen.`
export const radioUnusedLine = (n: number): string => (n === 1 ? '1 thing could still be built.' : `${n} things could still be built.`)

// Radio · Listening: starting a bounded run, and what each way it can end means. Every ending names its fix (UX rule: problems shown with their fix).
export const RADIO_LISTEN_TITLE = 'Listening'
export const RADIO_LISTEN_INTRO = 'Listens on one adapter, a short window on each channel in turn, for the hours you choose. It sends nothing. You confirm your passphrase once; it stops when the time is up or when you stop it.'
export const RADIO_LISTEN_ADAPTER = 'Adapter'
export const RADIO_LISTEN_HOURS = 'For'
export const RADIO_LISTEN_START = 'Start listening'
export const RADIO_LISTEN_STOP = 'Stop listening'
export const RADIO_LISTEN_NO_ADAPTER = 'No adapter can listen. Plug in a USB Wi-Fi adapter and look again.'
export const RADIO_LISTEN_OFF = 'Switch the Radio stack on in Modules before listening.'
export const RADIO_LISTEN_START_FAILED = 'Listening could not start.'
export const RADIO_LISTEN_HOUR_CHOICES = [1, 2, 4, 8, 12]
export const radioHoursLabel = (h: number): string => (h === 1 ? '1 hour' : `${h} hours`)
export const radioListeningUntil = (when: string): string => `Listening until ${when}.`
export const radioFramesLine = (frames: number): string => `${frames.toLocaleString()} frames heard so far.`
/** Where the run is: a cycle is one window on each channel of the plan. */
export const radioCycleLine = (cycles: number, heard: number, of: number): string =>
  heard >= of || heard === 0
    ? `${cycles} ${cycles === 1 ? 'cycle' : 'cycles'} done. ${cycles === 0 ? 'The first one is starting.' : 'Waiting for the next one.'}`
    : `Cycle ${cycles + 1} in progress: ${heard} of ${of} channels heard.${cycles ? ` ${cycles} ${cycles === 1 ? 'cycle' : 'cycles'} done.` : ''}`
export const RADIO_WINDOW_CUT = 'Stopped at the 4,000-frame limit: a busy channel, so counts here are a floor.'
export const RADIO_WINDOW_CUT_BADGE = 'Busy channel'
export const RADIO_WINDOW_MISSED = 'missed by the adapter'
export const RADIO_START_NEEDS = 'You will be asked for your passphrase once.' 
export const RADIO_WINDOWS_TITLE = 'Last windows'
export const RADIO_WINDOW_ON_CHANNEL = 'on the channel asked'
export const RADIO_WINDOW_OFF_CHANNEL = 'not on the channel asked'

/** Why a run ended, with what to do about it. The no_session case is the one most likely to be the helper not being set up. */
export const RADIO_ENDED: Record<string, { title: string; next: string }> = {
  no_session: { title: 'The helper would not listen.', next: 'Check Network, Settings, Privileged helper: it must be installed and radio_listen switched on. Then start again.' },
  time: { title: 'The time you chose is up.', next: 'Start again to keep listening.' },
  frames: { title: 'It heard more than it can keep up with and stopped.', next: 'This is a very busy place. Start again later, or listen on fewer channels.' },
  failing: { title: 'Windows kept failing.', next: 'Read the last error below, check the adapter is still plugged in, then start again.' },
  stopped: { title: 'You stopped it.', next: 'Start again when you want to listen.' },
  switched_off: { title: 'The Radio stack was switched off.', next: 'Switch it on in Modules to listen again.' },
  stack_stopped: { title: 'The service restarted.', next: 'Start again to keep listening.' },
}

// Radio · My devices: the owner names the devices that are theirs; everyone else's is only counted.
export const RADIO_DEVICES_TITLE = 'My devices'
export const RADIO_DEVICES_INTRO = 'Name the devices that are yours so they are told apart from your neighbours’. The address is kept sealed and is never shown again.'
export const RADIO_DEVICES_EMPTY = 'No device named yet.'
export const RADIO_DEVICE_ADDRESS = 'Address (like 3a:91:5c:d2:7e:44)'
export const RADIO_DEVICE_LABEL = 'Name'
export const RADIO_DEVICE_ADD = 'Add device'
export const RADIO_DEVICE_REMOVE = 'Remove'
export const RADIO_FORGET = 'Forget the neighbours'
export const RADIO_FORGET_BODY = 'Removes everything kept about other people’s devices and networks. Your own devices stay. Nothing already kept can be matched again.'
export const RADIO_FORGET_DONE = 'The neighbours are forgotten.'
export const RADIO_DEVICES_FAILED = 'Your devices could not be read.'

// Radio · Around you: what the air holds, as counts by class. Never a device of someone else.
export const RADIO_AROUND_TITLE = 'Around you'
export const RADIO_AROUND_EMPTY = 'Nothing heard yet. Start listening and the devices around you are counted here by kind.'
export const RADIO_CLASS_WORDS: Record<RadioClass, string> = {
  infrastructure: 'Always here',
  personal: 'Comes and goes',
  neighbour: 'Outside your walls',
  transient: 'Passing by',
  unknown: 'Not enough evidence yet',
}
export const RADIO_CLASS_HELP: Record<RadioClass, string> = {
  infrastructure: 'Heard in nearly every cycle with a steady signal: TVs, bulbs, plugs.',
  personal: 'Strong when here, and away part of the time: phones, watches, tags.',
  neighbour: 'Never strong: somewhere outside your four walls.',
  transient: 'Heard briefly, once: a passer-by or a car.',
  unknown: 'Too little listening so far, or no rule fits. Never a guess.',
}
export const RADIO_CLASS_ORDER: RadioClass[] = ['infrastructure', 'personal', 'neighbour', 'transient', 'unknown']
export const radioAroundTotal = (n: number): string => `${n} ${n === 1 ? 'device' : 'devices'} around you`
export const radioListenedLine = (cycles: number, need: number, days: number): string =>
  cycles >= need
    ? `Listened through ${cycles} cycles in the last ${days} days.`
    : `Listened through ${cycles} of the ${need} cycles needed before devices are sorted. Keep listening.`
export const RADIO_RULES_PROVISIONAL = 'The sorting rules are a first draft with cautious starting numbers, not yet fitted to your own air. Anything unclear stays “not enough evidence”.'
export const radioRulesLine = (version: number): string => `Sorting rules, version ${version}.`
export const radioProvisionalLine = (advanced: boolean, version: number): string => (advanced ? `${RADIO_RULES_PROVISIONAL} ${radioRulesLine(version)}` : RADIO_RULES_PROVISIONAL)
export const RADIO_CHANGES_TITLE = 'Recent changes in how devices were sorted'
export const RADIO_REASONS: Record<string, string> = {
  needs_more_listening: 'not enough listening yet',
  joins_your_network_always_here: 'joins your network and is always here',
  joins_your_network_comes_and_goes: 'joins your network and comes and goes',
  always_here_steady_signal: 'always here with a steady signal',
  brief_appearance: 'a brief appearance',
  never_strong: 'its signal was never strong',
  strong_and_comes_and_goes: 'strong, and comes and goes',
  no_rule_fits: 'no rule fits yet',
  not_heard_lately: 'sorted before, and not heard in the last few days',
}
export const radioChangeLine = (n: number, from: string, to: string, reason: string): string => {
  const what = `${n} ${n === 1 ? 'device' : 'devices'}`
  const why = RADIO_REASONS[reason] ?? reason
  const now = RADIO_CLASS_WORDS[to as RadioClass].toLowerCase()
  return from ? `${what}: ${RADIO_CLASS_WORDS[from as RadioClass].toLowerCase()} to ${now}, ${why}` : `${what} first sorted as ${now}, ${why}`
}

// Radio · Your networks: the names of your own Wi-Fi, and which access points are yours.
export const RADIO_NETWORKS_TITLE = 'Your networks'
export const RADIO_NETWORKS_INTRO = 'Name your own Wi-Fi networks. An access point that broadcasts one of the names is only a candidate: anyone can use the same name, so you confirm which ones are yours. A device that joins a confirmed one is sorted as yours.'
export const RADIO_NETWORK_NAME = 'Network name'
export const RADIO_NETWORK_ADD = 'Add name'
export const RADIO_NETWORK_NONE = 'No network named yet.'
export const RADIO_NETWORK_REMOVE_NAME = 'Remove name'
export const RADIO_AP_CONFIRM = 'Yes, this is mine'
export const RADIO_AP_REJECT = 'Not mine'
export const RADIO_AP_CONFIRMED = 'Confirmed as yours'
export const RADIO_AP_CANDIDATE = 'Candidate: confirm it, or say it is not yours'
export const radioApLine = (name: string, channel: number, security: string, seen: string): string => `${name} · channel ${channel} · ${security || 'security unknown'} · last heard ${seen}`
export const RADIO_NETWORKS_FAILED = 'Your networks could not be read.'
export const RADIO_CLASS_PICK = 'Kind of device (optional)'
export const RADIO_CLASS_BY_OWNER = 'your word'
export const RADIO_CLASS_BY_RULES = 'decided by the rules'

/** The band a Wi-Fi channel number belongs to. The plan only offers channels of 2.4 and 5 GHz. */
export const radioBand = (channel: number): string => (channel <= 14 ? '2.4 GHz' : '5 GHz')
export const radioChannelWord = (channel: number): string => `Channel ${channel}`
export const radioWindowOwn = (n: number): string => `${n} from your devices`
export const radioWindowFrames = (n: number): string => `${n.toLocaleString()} ${n === 1 ? 'frame' : 'frames'}`
export const radioWindowsHint = (cycleSeconds: number, windowSeconds: number): string => `One window of ${windowSeconds} seconds on each channel in turn, a new cycle every ${Math.round(cycleSeconds / 60)} minutes.`
export const RADIO_WINDOWS_EMPTY = 'No window has finished yet. The first one takes about 20 seconds.'

// Radio · Neighbourhood: other people's devices, by kind, each kind opening onto its devices. A device is a short id and words, never an address.
export const RADIO_NEIGH_LINK = 'See who is around'
export const RADIO_TAB_LINK_AROUND = 'Open the Around you tab to see who is around'
export const radioNeighNearUnknown = (n: number): string => `${n} ${n === 1 ? 'device that is not yet sorted has' : 'devices that are not yet sorted have'} a strong signal: close to the house.`
export const RADIO_SIGNAL_WORDS: Record<RadioSignal, string> = { strong: 'Strong', medium: 'Medium', weak: 'Weak', unknown: 'Not measured' }
export const RADIO_SIGNAL_HELP: Record<RadioSignal, string> = {
  strong: 'close: probably inside the house',
  medium: 'in between',
  weak: 'far, or behind walls',
  unknown: 'no signal was measured',
}
export const RADIO_DEVICE_COLUMNS = { id: 'Device', signal: 'Signal', heard: 'Heard in', retries: 'Retries', channels: 'Channels', last: 'Last heard' } as const
export const radioHeardLine = (heard: number, listened: number): string => `${heard} of ${listened} cycles`
export const radioRetryWord = (percent: number): string => `${percent}%`
export const radioChannelsLine = (channels: number[] | null): string => (channels?.length ? channels.join(', ') : 'none')
export const RADIO_RANDOM_ADDRESS = 'Uses a changing address, like a phone does'
export const RADIO_JOINED_OWN = 'Joined your network'
/** What a device that is still unsorted is waiting for: more cycles heard, and more cycles listened to overall. */
export const radioWaitingLine = (heard: number, needHeard: number, listened: number, needListened: number): string => {
  const parts: string[] = []
  if (listened < needListened) parts.push(`the sensor has listened through ${listened} of the ${needListened} cycles it needs`)
  if (heard < needHeard) parts.push(`this device has been heard in ${heard} of the ${needHeard} cycles it needs`)
  return parts.length ? `Waiting: ${parts.join(', and ')}.` : 'No rule fits it yet: it stays unsorted rather than guessed.'
}
/** How far along the evidence is, 0 to 100, for the bar of an unsorted device. */
export const radioWaitingPercent = (heard: number, needHeard: number, listened: number, needListened: number): number =>
  Math.round(100 * Math.min(1, Math.min(heard / Math.max(needHeard, 1), listened / Math.max(needListened, 1))))

// Radio · Airwaves: how the channels and the kinds of device use the air. Counts only.
export const RADIO_AIR_TITLE = 'Airwaves'
export const RADIO_AIR_INTRO = 'How crowded each channel is, and which kinds of device waste the most air on retries. Counts only: no neighbour is listed.'
export const RADIO_AIR_CHANNELS = 'Devices heard on each channel'
export const RADIO_AIR_CLASSES = 'Retries by kind of device'
export const RADIO_AIR_CLASSES_HELP = 'A retry is a frame sent again because the first was not acknowledged. A high share means a weak link or a crowded channel.'
export const RADIO_AIR_EMPTY = 'Nothing counted yet. Start listening and the channels fill in.'
export const RADIO_AIR_FAILED = 'The airwaves could not be read.'
export const radioAirRouter = (name: string, channel: number): string => `${name}: channel ${channel}`
export const radioAirBusiest = (channel: number, devices: number): string => `Busiest: channel ${channel}, ${devices} ${devices === 1 ? 'device' : 'devices'}.`
export const radioAirClassLine = (devices: number, frames: number, percent: number): string => `${devices} ${devices === 1 ? 'device' : 'devices'}, ${frames.toLocaleString()} frames, ${percent}% retried`
export const radioAirFramesLine = (devices: number, frames: number): string => `${devices} ${devices === 1 ? 'device' : 'devices'}, ${frames.toLocaleString()} frames`
export const RADIO_AIR_CAP_NOTE = 'On a busy channel a window stops at 4,000 frames, so counts there are a floor.'

// Radio · My devices: where each device is, hour by hour.
export const RADIO_PRESENCE_TITLE = 'Where your devices are'
export const RADIO_PRESENCE_INTRO = 'Each square is one hour of the last two days. Filled means the sensor heard the device; outlined means it listened and did not hear it; pale means it was not listening, so nothing is known.'
export const RADIO_PRESENCE_EMPTY = 'Add a device below and start listening to see when it is here.'
export const RADIO_HOUR_WORDS: Record<RadioHourState, string> = { here: 'here', away: 'not heard', not_listening: 'not listening' }
export const RADIO_NOW_WORDS: Record<RadioHourState, string> = { here: 'Here now', away: 'Not heard lately', not_listening: 'Not listening, so not known' }
export const radioLastHeardLine = (when: string): string => `Last heard ${when}`
export const RADIO_NEVER_HEARD = 'Never heard yet'
export const radioHourTitle = (when: string, state: RadioHourState, signal?: RadioSignal): string =>
  `${when}: ${RADIO_HOUR_WORDS[state]}${state === 'here' && signal && signal !== 'unknown' ? `, ${RADIO_SIGNAL_WORDS[signal].toLowerCase()} signal` : ''}`
export const RADIO_JOINED_EVIDENCE = 'Has joined one of your networks'

// Detail level (the same Simple and Advanced as the Network screens).
export const RADIO_DETAIL_LABEL = 'How much detail'

// Radio · Findings: what the stack found, in its own fixed words, and what it could not check.
export const RADIO_FINDINGS_TITLE = 'Things to look at'
export const RADIO_FINDINGS_NONE = 'Nothing to look at. The checks that can run found nothing.'
export const RADIO_FINDINGS_THIN = 'Not much has been heard yet, so this says little. Keep listening.'
export const RADIO_FINDINGS_FAILED = 'The findings could not be read.'
export const RADIO_TIER_WORDS: Record<RadioTier, string> = { seen: 'Seen', inferred: 'Worked out', estimated: 'Estimated' }
export const RADIO_TIER_HELP: Record<RadioTier, string> = {
  seen: 'counted directly from what was heard',
  inferred: 'worked out by a rule over what was heard',
  estimated: 'a rough guess from signal strength',
}
export const radioFindingCount = (n: number): string => (n === 1 ? '1 case' : `${n} cases`)
export const radioUnseenLine = (n: number): string => `${n} ${n === 1 ? 'check cannot' : 'checks cannot'} run yet, so a quiet list does not mean they are clear.`
export const RADIO_UNSEEN_TITLE = 'Checks that cannot run yet'

// Radio · the radar: what the sensor hears now, placed by signal strength. One adapter measures how strong, not which way.
export const RADIO_RADAR_EMPTY = 'Nothing is being heard. Start listening on the Listening tab and the devices appear here.'
export const RADIO_LIVE_FAILED = 'The live picture could not be read.'
export const RADIO_CALIBRATE_FAILED = 'The distances could not be calibrated.'
export const RADIO_RADAR_LABEL = 'Radar of devices around the computer'
export const RADIO_RING_WORDS: Record<RadioRing, string> = { room: 'In this room', house: 'In the house', nearby: 'Next door or the street', far: 'Too weak to place' }
export const RADIO_RING_ORDER: RadioRing[] = ['room', 'house', 'nearby', 'far']
export const RADIO_TREND_WORDS: Record<RadioTrend, string> = { closer: 'getting closer', away: 'moving away', steady: 'steady', unknown: 'too new to tell' }
export const RADIO_TREND_MARK: Record<RadioTrend, string> = { closer: '▲', away: '▼', steady: '•', unknown: '' }
export const RADIO_RADAR_MINE = 'Yours'
export const RADIO_RADAR_NOT_HEARD = 'not heard this cycle'
export const RADIO_RADAR_PROVISIONAL = 'Distances come from a starting guess for how fast a signal fades, so read them as rough rings, not metres. Stand one of your devices one metre from this computer and press Calibrate to measure from it.'
export const RADIO_RADAR_CALIBRATED = 'Calibrated against your device at one metre. Still a range: walls and antennas move it.'
export const RADIO_RADAR_CALIBRATE = 'Calibrate: this device is 1 m from the computer'
export const radioRangeLine = (near: number, far: number): string => (near >= far - 0.05 ? `about ${near} m` : `${near}–${far} m`)
export const radioDotLabel = (d: { mine: boolean; label?: string; id: string }): string => (d.mine ? d.label || d.id : d.id)
export const radioRadarSummary = (n: number, mine: number): string => `${n} ${n === 1 ? 'device' : 'devices'} heard now${mine ? `, ${mine} of them yours` : ''}`
export const RADIO_RETRIES_NOT_REPORTED = 'This adapter reports no retried frames at all (0 of every frame heard), so retry shares are not shown. The code reads the flag correctly in its tests, so the adapter or its driver may be hiding it: not yet confirmed.'

// Radio · Scan plan: which channels a run visits, how long it stays on each and how often it comes round.
export const RADIO_SCAN_TITLE = 'Scan plan'
export const RADIO_SCAN_INTRO = 'A run visits each chosen channel for a short window, then comes round again. More channels or shorter cycles hear more and ask the helper for more windows. A new plan takes effect the next time you start listening, and that start asks for your passphrase again.'
export const RADIO_SCAN_CHANGE = 'Change the plan'
export const RADIO_SCAN_CLOSE = 'Close'
export const RADIO_SCAN_CHANNELS = 'Channels to visit'
export const RADIO_SCAN_WINDOW = 'Time on each channel'
export const RADIO_SCAN_CYCLE = 'Come round every'
export const RADIO_SCAN_SAVE = 'Save plan'
export const RADIO_SCAN_RESET = 'Use the default plan'
export const RADIO_SCAN_SAVED = 'Saved. It applies the next time you start listening.'
export const RADIO_SCAN_FAILED = 'The scan plan could not be read.'
export const RADIO_SCAN_SAVE_FAILED = 'The scan plan could not be saved.'
export const RADIO_SCAN_RUNNING = 'A run is listening now. Stop it to change the plan.'
export const RADIO_SCAN_NONE = 'Choose at least one channel.'
export const RADIO_SCAN_WINDOW_CHOICES = [5, 10, 20, 30]
export const RADIO_SCAN_CYCLE_CHOICES = [60, 120, 300, 600, 900, 1800]
export const radioSecondsLabel = (s: number): string => (s % 60 === 0 ? `${s / 60} ${s === 60 ? 'minute' : 'minutes'}` : `${s} seconds`)
export const radioScanSummary = (channels: number, window: number, cycle: number, perHour: number, custom: boolean): string =>
  `${channels} ${channels === 1 ? 'channel' : 'channels'}, ${window} seconds each, every ${radioSecondsLabel(cycle)}: ${perHour} windows an hour${custom ? '' : ' (the default)'}.`
export const radioScanTooMany = (max: number): string => `That is more than ${max} windows an hour. Choose fewer channels or a longer cycle.`
export const RADIO_SCAN_NO_FIT = 'The windows do not fit in one cycle. Shorten them, choose fewer channels or a longer cycle.'

// Radio · usual positions and changes from them (the stored days, day resolution).
export const RADIO_RADAR_USUAL_LEGEND = 'Hollow dots are devices usually here that are not heard now, placed by their average over the days kept. They show where things usually are, not where they were at any moment.'
export const radioUsualMore = (n: number): string => `${n} ${n === 1 ? 'device is' : 'devices are'} usually here and not heard now.`
export const RADIO_RADAR_USUAL_SHOW = 'Show them'
export const RADIO_RADAR_USUAL_HIDE = 'Hide them'
export const RADIO_RADAR_USUAL_ROW = 'usually here'
export const radioShiftWord = (shift: number, usual: number): string =>
  shift > 0 ? `${shift} dB stronger than its usual ${usual} dBm: closer than usual` : `${-shift} dB weaker than its usual ${usual} dBm: farther than usual`
export const radioUnusualSummary = (n: number): string => `${n} ${n === 1 ? 'is' : 'are'} unlike ${n === 1 ? 'its' : 'their'} usual.`
export const RADIO_PRESENCE_DISTANCE = 'Distance over the same hours: the bar is the range it could be in, from the radar’s model.'
export const RADIO_PRESENCE_CLOSER = 'closer'
export const RADIO_PRESENCE_FARTHER = 'farther'

export const RADIO_TRACK_TITLE = 'Distance over time'
export const RADIO_TRACK_FAILED = 'The track could not be read.'
export const RADIO_TRACK_NONE = 'No track yet: this device has not been heard in a finished cycle.'
export const RADIO_TRACK_NOTE_MEMORY = 'Kept in memory only and forgotten when listening stops. The band is the range the distance can be in.'
export const RADIO_TRACK_NOTE_DISK = 'Kept on this computer for the time you chose under What is kept. The band is the range the distance can be in.'
export function radioTrackSummary(heard: number, spanSeconds: number): string {
  const mins = Math.max(1, Math.round(spanSeconds / 60))
  const span = mins >= 90 ? `${Math.round(mins / 60)} hours` : `${mins} minutes`
  return `Heard ${heard} times in the last ${span}`
}

export const RADIO_KEEP_TITLE = 'What is kept'
export const RADIO_KEEP_INTRO = 'Other people’s devices are kept as a one-way code with a kind and a signal band, by the day, for 14 days. Nothing finer is written to disk unless you turn the track on.'
export const RADIO_KEEP_TRACK_LABEL = 'Signal track on disk'
export const RADIO_KEEP_CHOICES = [
  { value: '0', label: 'Off' },
  { value: '24', label: '24 hours' },
  { value: '48', label: '48 hours' },
]
export const RADIO_KEEP_OFF_NOTE = 'Off: a track is drawn from memory while listening and is gone when it stops.'
export const RADIO_KEEP_ON_NOTE = 'On: each device’s signal is kept every cycle, so a track survives a restart. It holds a one-way code, a time and a signal, never an address, and it is deleted when it is older than the time you chose.'
export const RADIO_KEEP_FAILED = 'What is kept could not be read or changed.'
export const RADIO_KEEP_DROP = 'Delete stored tracks'
export const RADIO_KEEP_DROPPED = 'The stored tracks are deleted.'
export const RADIO_KEEP_TURNED_OFF = 'Tracks are off and the stored ones are deleted.'
export function radioKeepStored(points: number, oldest?: string): string {
  if (!points) return 'Nothing is stored yet.'
  const since = oldest ? `, the oldest from ${new Date(oldest).toLocaleString()}` : ''
  return `${points.toLocaleString()} points stored${since}.`
}

// Radio · Around you: the status line, the filter chips and the chosen device.
export const RADIO_STATUS_IDLE = 'Not listening. Showing where devices usually are.'
export const RADIO_STATUS_START = 'Start listening'
export const RADIO_STATUS_READING = 'Reading the air now'
export const radioStatusListening = (secondsLeft: number): string => {
  const m = Math.floor(secondsLeft / 60)
  const sec = String(secondsLeft % 60).padStart(2, '0')
  return `Listening. Next reading in ${m}:${sec}`
}
export const radioStatusSorting = (done: number, need: number): string => `Sorting starts after ${need} cycles (${done} done)`
export const RADIO_FILTER_LABEL = 'Show devices'
export const RADIO_FILTER_ALL = 'All'
export const RADIO_FILTER_MINE = 'Yours'
export const RADIO_CHIP_WORDS: Record<RadioClass, string> = {
  infrastructure: 'Always here', personal: 'Comes and goes', neighbour: 'Outside', transient: 'Passing by', unknown: 'Not sorted',
}
export const RADIO_LIST_NONE = 'No device in this group.'
export const radioListMore = (n: number): string => `Show ${n} more`
export const RADIO_LIST_LESS = 'Show fewer'
export const RADIO_DETAIL_CLOSE = 'Close'
export const RADIO_DETAIL_WHY = 'Why this kind'
export const RADIO_DETAIL_SIGNAL = 'Signal'
export const RADIO_DETAIL_LAST = 'Last heard'
export const RADIO_DETAIL_FIRST = 'First heard'
export const RADIO_GUIDE_TITLE = 'How to read this'
export const RADIO_GUIDE_DIRECTION = 'Which way a device lies is not measured with one adapter, so a dot keeps a steady place on its ring. Only how far is estimated, as a range, from how strong its signal is.'
export const RADIO_GUIDE_MOTION = 'A dot moves once a cycle, when the sensor has been round every channel. A shorter cycle in the scan plan gives more frequent movement.'
export const RADIO_LISTEN_MANUAL = 'A run lasts the hours you choose, up to 12, then ends by itself. Nothing starts a run on its own, and a restart of the service ends one that is going.'
