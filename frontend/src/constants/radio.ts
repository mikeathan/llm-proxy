import type { CapabilityUse, RadioKind } from '../types/radio'

// The words of the Radio · Sensors page. Capability titles and notes come from the server; these are the fixed phrases around them.

export const RADIO_SENSORS_TITLE = 'Sensors'
export const RADIO_SENSORS_INTRO = 'Each radio found on this computer, from the system’s own records. Reading this page sends nothing and listens to nothing.'
export const RADIO_OFF_TITLE = 'The Radio stack is off'
export const RADIO_OFF_BODY = 'Nothing listens until you switch it on in Modules. Until then this page only lists what is plugged in.'
export const RADIO_OFF_ACTION = 'Open Modules'
export const RADIO_EMPTY_TITLE = 'No radio to listen with'
export const RADIO_EMPTY_BODY = 'Plug in a USB Wi-Fi adapter and reload. With no adapter the stack does nothing at all.'
export const RADIO_RELOAD = 'Look again'
export const RADIO_LOAD_FAILED_TITLE = 'The radios could not be read'
export const RADIO_LOAD_NEXT = 'Check that the server is running, then retry.'

export const RADIO_KIND_WORDS: Record<RadioKind, string> = {
  wifi: 'Wi-Fi',
  bluetooth: 'Bluetooth',
  ieee802154: 'Zigbee / Thread',
}

export const RADIO_USE_WORDS: Record<CapabilityUse, string> = {
  used: 'In use',
  unused_by_design: 'Not used, by design',
  unused_not_built: 'Not used yet',
  found_no_driver: 'Found, no driver',
}

export const RADIO_ELIGIBLE_WORD = 'Can listen'
export const RADIO_NOT_ELIGIBLE_WORD = 'Will not listen'
export const RADIO_CAPABILITIES_TITLE = 'What it can do'
export const RADIO_NOT_READ_YET = 'not read yet'

export const radioUnusedLine = (n: number): string => (n === 1 ? '1 capability is found and not used yet.' : `${n} capabilities are found and not used yet.`)

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
export const radioCycleLine = (cycles: number, frames: number): string => `${cycles} ${cycles === 1 ? 'cycle' : 'cycles'} done, ${frames} frames heard so far.`
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
