import {
  RADIO_FIRST_LOOK, RADIO_KIND_WORDS, RADIO_MY_DEVICE, RADIO_OWN_HOUSE, RADIO_OWN_MINE, RADIO_OWN_NEIGHBOUR, RADIO_OWN_NEIGHBOUR_ROUTER, RADIO_OWN_ON_WIFI, RADIO_OWN_PASSING,
  RADIO_OWN_PERSONAL, RADIO_OWN_ROUTER_RADIO, RADIO_OWN_THIS_COMPUTER, RADIO_OWN_UNSURE, RADIO_OWN_YOUR_ROUTER, RADIO_UNKNOWN_DEVICE, radioBrandKind, radioKindPrivate,
  radioOwnChanging, radioOwnOnLAN, radioSureWord, radioThisComputer,
} from '../../constants/radioIdentity'
import type { Nameable, RadioClass, RadioDeviceKind, RadioIdent, RadioOwnership } from '../../types/radio'

// Radio · how a device is named and what the page concludes about it (ADR-0058). Pure functions over what the server sent; no state.
// The short id (four bytes of a one-way tag) is a handle shown small beside the name, never the name itself.

const kindOf = (d: Nameable): RadioDeviceKind => d.ident?.kind ?? 'unknown'
const privateAddress = (d: Nameable): boolean => !!d.detail?.random_address || d.ident?.kind_source === 'private_address'

/** The brand and model together, without saying the brand twice. */
export function brandModel(id: RadioIdent | undefined): string {
  if (!id) return ''
  const brand = id.brand_hint ? '' : (id.brand ?? '')
  const model = id.model ?? ''
  if (!model) return ''
  if (!brand || model.toLowerCase().startsWith(brand.toLowerCase().split(' ')[0] ?? '')) return model
  return `${shortBrand(brand)} ${model}`
}

// The city many makers put first in their registered name ("Hui Zhou Gaoshengda", "Shenzhen Century Xinyang"): the city is not the maker.
const CITY_PREFIX = /^(hui zhou|huizhou|shenzhen|guangzhou|dongguan|hangzhou|shanghai|beijing|xiamen|zhuhai|suzhou|ningbo|fujian|guangdong|zhongshan|foshan|chengdu|wuhan|qingdao|tianjin|nanjing)\s+/i

/** A maker's name without the city it starts with and the company words ("ASUSTek COMPUTER INC." reads "ASUSTek", "Hui Zhou Gaoshengda"
 * reads "Gaoshengda"). */
export function shortBrand(brand: string): string {
  const named = brand.trim().replace(CITY_PREFIX, '')
  const cut = named.replace(/[,.]/g, ' ').split(/\s+/).filter((w) => w && !/^(inc|ltd|limited|corp|corporation|co|company|computer|technologies|technology|electronics|international|gmbh|sa|ag|plc|llc)$/i.test(w))
  return cut.slice(0, 2).join(' ') || brand
}

/** The name a device goes by, strongest first: your name for it, its name on your network, its make and model, its maker and kind, the network
 * it broadcasts (shown under the name otherwise), its maker, then its kind. Never its short id. */
export function nameOf(d: Nameable): string {
  if (d.mine && d.label) return d.label
  const id = d.ident
  if (id?.this_computer) return radioThisComputer(id.this_computer)
  if (id?.lan_name && !id.router_radio) return id.lan_name
  const bm = brandModel(id)
  if (bm) return bm
  const kind = RADIO_KIND_WORDS[kindOf(d)]
  if (id?.brand && kindOf(d) !== 'unknown') return radioBrandKind(shortBrand(id.brand), kind)
  if (d.network) return d.network
  if (id?.brand) return shortBrand(id.brand)
  if (d.detail?.maker) return kindOf(d) === 'unknown' ? shortBrand(d.detail.maker) : radioBrandKind(shortBrand(d.detail.maker), kind)
  if (privateAddress(d)) return radioKindPrivate(kind)
  if (kindOf(d) !== 'unknown') return kind
  return RADIO_UNKNOWN_DEVICE
}

/** Whose it is, by the evidence: yours (your word), likely yours (the frames or your network say so), not yours, or unknown. */
export function ownershipOf(d: Nameable): RadioOwnership {
  if (d.mine) return 'mine'
  const id = d.ident
  if (id && (id.this_computer || id.on_your_wifi || id.your_ap || id.box_yours || id.router_radio || (id.lan_name && !id.router_radio) || id.net_yours)) return 'likely'
  if (d.class === 'neighbour' || d.class === 'transient' || (id?.net && !id.net_yours)) return 'neighbour'
  return 'unknown'
}

/** The one-line conclusion a row leads with: whose it is and what it is, with how sure. */
export function conclusionOf(d: Nameable): string {
  const id = d.ident
  if (d.mine) return RADIO_OWN_MINE
  if (id?.this_computer) return RADIO_OWN_THIS_COMPUTER
  if (id?.your_ap) return RADIO_OWN_YOUR_ROUTER
  if (id?.box_yours || id?.router_radio) return RADIO_OWN_ROUTER_RADIO
  if (id?.lan_name) return radioOwnOnLAN(id.lan_name)
  if (id?.on_your_wifi) return RADIO_OWN_ON_WIFI
  if (id?.pending && id.pending !== d.class) return radioOwnChanging(classPhrase(d, id.pending))
  if (d.class === 'unknown') return id?.first_look ? RADIO_FIRST_LOOK[id.first_look] : RADIO_OWN_UNSURE
  return `${radioSureWord(id?.confidence ?? 0)}: ${classPhrase(d, d.class).toLowerCase()}`
}

function classPhrase(d: Nameable, c: RadioClass): string {
  const router = kindOf(d) === 'router'
  switch (c) {
    case 'neighbour': return router ? RADIO_OWN_NEIGHBOUR_ROUTER : RADIO_OWN_NEIGHBOUR
    case 'transient': return RADIO_OWN_PASSING
    case 'infrastructure': return RADIO_OWN_HOUSE
    case 'personal': return RADIO_OWN_PERSONAL
    default: return RADIO_OWN_UNSURE
  }
}

/** Whether a device is a router (or one of its radios). */
export const isRouter = (d: Nameable): boolean => kindOf(d) === 'router' || !!d.ident?.your_ap || (d.ident?.clients ?? 0) > 0


/** The evidence for whose a device is, as plain sentences, strongest first. */
export function evidenceOf(d: Nameable, words: {
  yourAP: string; boxYours: string; router: string; lan: string; netYours: string; netOther: string; none: string; onWiFi: (n: number) => string
}): string[] {
  const id = d.ident
  if (!id) return [words.none]
  const out: string[] = []
  if (id.your_ap) out.push(words.yourAP)
  if (id.box_yours && !id.your_ap) out.push(words.boxYours)
  if (id.router_radio) out.push(words.router)
  if (id.lan_name && !id.router_radio) out.push(words.lan)
  if (id.on_your_wifi) out.push(words.onWiFi(id.own_cycles ?? 0))
  else if (id.net && id.net_yours) out.push(words.netYours)
  else if (id.net) out.push(words.netOther)
  return out.length ? out : [words.none]
}

/** The label offered when you say a device is yours: its make and model, its maker, or "My device". Never its kind: that is a guess shown beside
 * the name, where it can change, and a label is kept (two Macs guessed phones were saved as "Phone or tablet", 2026-11-18). Never its name on
 * your network or the network it broadcasts: those are kept off the disk (ADR-0058, Constitution I.16), and a label is written to it. You can
 * type anything instead, and rename it later. */
export function defaultLabel(d: Nameable): string {
  const id = d.ident
  const bm = brandModel(id)
  if (bm) return bm
  const brand = id?.brand ?? d.detail?.maker
  return brand ? shortBrand(brand) : RADIO_MY_DEVICE
}
