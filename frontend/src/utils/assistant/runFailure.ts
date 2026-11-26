import type { RunFailure } from '../../types/assistant'

// describeRunFailure turns a stored run error into a sentence and a next step (owner's rule: a plain sentence plus the
// fix, the raw text behind Details). The first rule whose pattern matches wins; the raw text is always kept as Details.
const RULES: { pattern: RegExp; failure: RunFailure }[] = [
  {
    pattern: /x-opencode-session|MissingSessionID/i,
    failure: {
      title: "The model's provider turned the request down.",
      hint: 'OpenCode wants a session header on every request. Add the key under OpenCode Go in Models › Providers and use the model from there.',
      fixInModels: true,
    },
  },
  {
    pattern: /prompt too large|context window|context length|too many tokens/i,
    failure: {
      title: "The conversation no longer fits this model's window.",
      hint: 'Start a new chat, or pick a model with a larger window.',
      fixInModels: false,
    },
  },
  {
    pattern: /HTTP 401|HTTP 403|unauthori[sz]ed|invalid api key|forbidden/i,
    failure: { title: 'The provider refused the key.', hint: 'Check the key in Models › Providers.', fixInModels: true },
  },
  {
    pattern: /HTTP 429|rate.?limit|too many requests/i,
    failure: { title: 'The provider is limiting requests.', hint: 'Wait a minute and try again, or pick another model.', fixInModels: false },
  },
  {
    pattern: /port .*in use|would not start|failed to become ready|cannot start local model/i,
    failure: { title: 'The local model would not start.', hint: 'Its log on the Models page says why.', fixInModels: true },
  },
  {
    pattern: /timed? ?out|deadline exceeded/i,
    failure: { title: 'The model took too long to answer.', hint: 'Try again; if it keeps happening, give the model a longer timeout in Models.', fixInModels: true },
  },
]

const FALLBACK: RunFailure = { title: 'The run failed.', hint: 'Try again, or pick another model.', fixInModels: false }

export function describeRunFailure(message: string): RunFailure {
  return RULES.find((r) => r.pattern.test(message))?.failure ?? FALLBACK
}
