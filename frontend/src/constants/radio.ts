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
