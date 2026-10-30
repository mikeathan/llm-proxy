import { DECOYS_NONE } from '../../constants/netaudit'
import type { DecoyTouch } from '../../types/netaudit'

/** The one line a folded list of touches shows: how many devices connected to a decoy (one device on two decoys counts once). */
export function touchSummary(touches: DecoyTouch[]): string {
  if (!touches.length) return DECOYS_NONE
  const devices = new Set(touches.map((t) => t.source)).size
  return `${devices} ${devices === 1 ? 'device' : 'devices'} connected`
}
