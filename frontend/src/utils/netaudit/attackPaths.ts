import type { AttackPathPrize, AttackPathStep } from '../../types/netaudit'

const ACCEPTED_WORD = 'accepted'

/** The end of a path in plain words. A device the person marked important is shown by their name for it, else by its address. */
export function prizeText(prize: AttackPathPrize): string {
  switch (prize.kind) {
    case 'computer':
      return 'This computer'
    case 'router':
      return 'Your router'
    case 'important':
      return prize.name || `The device at ${prize.where}`
  }
}

/** One weakness on the way: its title, its port, and the word "accepted" when the person accepted the finding. */
export function stepText(step: AttackPathStep): string {
  const port = step.port ? ` (port ${step.port})` : ''
  return `${step.title}${port}${step.accepted ? ` · ${ACCEPTED_WORD}` : ''}`
}

/** How many of the listed paths the one fix breaks; nothing to say when it is only its own. */
export function breaksLine(breaks: number): string {
  return breaks > 1 ? `This one fix breaks ${breaks} of these paths.` : ''
}
