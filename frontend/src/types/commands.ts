// Slash commands as GET /admin/api/commands, POST /admin/api/commands/run and /preview report them.
export interface CommandInfo {
  name: string
  usage: string
  summary: string
  examples: string[]
}

// The chat turn a turn command (/research, /agent) asks for: send message under agent, with helpers when set.
export interface CommandTurn {
  message: string
  agent?: string
  use_helpers?: boolean
}

export interface CommandRunResult {
  // False for text that is not a command: send it to the assistant as usual.
  handled: boolean
  reply: string
  // True when the command was refused; the reply says why.
  failed: boolean
  // Set by a turn command: the chat sends this turn instead of answering with the reply alone.
  turn?: CommandTurn
}

export interface CommandPreview {
  matched: boolean
  command?: string
  // What the command would do with the text so far.
  text?: string
  // Why it would be refused.
  error?: string
}

// The preview shown under the chat box while a command is typed: what it would do, or why it would be refused.
export interface SlashPreview {
  text: string
  error: string
}
