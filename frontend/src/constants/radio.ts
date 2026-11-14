import type { CapabilityUse, RadioClass, RadioKind, RadioRole } from '../types/radio'
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
export const RADIO_WINDOW_CUT = 'cut short at the 4,000-frame limit: a busy channel, so counts here are a floor'
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
