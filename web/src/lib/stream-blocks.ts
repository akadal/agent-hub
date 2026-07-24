/**
 * Terminal stream → chat/history blocks.
 * Pure presentation transform over raw PTY text (ANSI stripped for display).
 *
 * Thinking/reasoning regions are best-effort via common stream markers.
 * Slash-command lines (content starting with `/`) stay visible.
 */

export type StreamBlockKind = 'output' | 'thinking' | 'command' | 'system'

export interface StreamBlock {
  id: string
  kind: StreamBlockKind
  text: string
  /** True when this region can collapse (thinking / intermediate reasoning). */
  collapsible: boolean
}

/** CSI / OSC / simple ANSI escape sequences. */
const ANSI_RE =
  // eslint-disable-next-line no-control-regex
  /(?:\u001b\[[0-9;?]*[ -/]*[@-~]|\u001b\][^\u0007\u001b]*(?:\u0007|\u001b\\)|\u001b[()][0-9A-Za-z]|\u001b[=>]|\u001b[NO]|\r(?!\n))/g

export function stripAnsi(input: string): string {
  return input.replace(ANSI_RE, '').replace(/\r\n/g, '\n').replace(/\r/g, '\n')
}

/** Open/close markers for collapsible thinking-style regions. */
const THINKING_OPEN =
  /^(?:<\s*thinking\s*>|\[\s*thinking\s*\]|thinking\s*:|reasoning\s*:|thought\s*:|thinking\s*\.{0,3}\s*$)/i
const THINKING_CLOSE =
  /^(?:<\/\s*thinking\s*>|\[\s*\/\s*thinking\s*\]|end\s+thinking\b)/i

const SYSTEM_LINE =
  /^\[(?:connection lost|reconnected|session closed|ssh ready)[^\]]*\]$/i

function isSlashCommandLine(line: string): boolean {
  const t = line.trimStart()
  // Keep slash commands visible: /help, /status, /model, etc.
  // Avoid matching paths like /usr/bin or // comments.
  if (!t.startsWith('/')) return false
  if (t.startsWith('//')) return false
  if (t.startsWith('/*')) return false
  // Absolute paths: /foo/bar
  if (/^\/[A-Za-z0-9._-]+\//.test(t)) return false
  // /command or /command args
  return /^\/[A-Za-z][\w-]*(?:\s|$)/.test(t)
}

function makeId(kind: StreamBlockKind, index: number, salt: string): string {
  return `${kind}-${index}-${salt.length}`
}

/**
 * Parse a full (or accumulated) terminal stream into history-like blocks.
 */
export function parseStreamToBlocks(raw: string): StreamBlock[] {
  const plain = stripAnsi(raw)
  if (!plain.trim()) return []

  const lines = plain.split('\n')
  const blocks: StreamBlock[] = []
  let buf: string[] = []
  let kind: StreamBlockKind = 'output'
  let index = 0

  const flush = () => {
    const text = buf.join('\n').replace(/^\n+|\n+$/g, '')
    if (!text.trim()) {
      buf = []
      return
    }
    const collapsible = kind === 'thinking'
    blocks.push({
      id: makeId(kind, index++, text.slice(0, 24)),
      kind,
      text,
      collapsible,
    })
    buf = []
  }

  let inThinking = false

  for (const line of lines) {
    const trimmed = line.trim()

    if (inThinking) {
      if (THINKING_CLOSE.test(trimmed) || trimmed === '') {
        // Close on explicit end marker; blank line ends soft "thinking:" sections.
        if (THINKING_CLOSE.test(trimmed)) {
          // drop close marker line
          flush()
          inThinking = false
          kind = 'output'
          continue
        }
        // blank: end soft thinking block
        if (trimmed === '' && buf.length > 0) {
          flush()
          inThinking = false
          kind = 'output'
          continue
        }
      }
      buf.push(line)
      continue
    }

    if (THINKING_OPEN.test(trimmed)) {
      flush()
      inThinking = true
      kind = 'thinking'
      // Keep marker content if it's "thinking: body" on one line
      const soft = trimmed.match(/^(?:thinking|reasoning|thought)\s*:\s*(.*)$/i)
      if (soft && soft[1]) {
        buf = [soft[1]]
      } else if (/^<\s*thinking\s*>/i.test(trimmed)) {
        const rest = trimmed.replace(/^<\s*thinking\s*>\s*/i, '')
        buf = rest ? [rest] : []
      } else if (/^\[\s*thinking\s*\]/i.test(trimmed)) {
        const rest = trimmed.replace(/^\[\s*thinking\s*\]\s*/i, '')
        buf = rest ? [rest] : []
      } else {
        buf = []
      }
      continue
    }

    if (SYSTEM_LINE.test(trimmed)) {
      flush()
      kind = 'system'
      buf = [line]
      flush()
      kind = 'output'
      continue
    }

    if (isSlashCommandLine(line)) {
      flush()
      kind = 'command'
      buf = [line]
      flush()
      kind = 'output'
      continue
    }

    if (kind !== 'output') {
      flush()
      kind = 'output'
    }
    buf.push(line)
  }

  flush()
  return blocks
}

/**
 * Incremental helper: append a chunk and re-parse the full accumulated stream.
 * Returns updated raw buffer + blocks (UI can replace the list each time).
 */
export function appendStreamChunk(
  prevRaw: string,
  chunk: string,
): { raw: string; blocks: StreamBlock[] } {
  const raw = prevRaw + chunk
  return { raw, blocks: parseStreamToBlocks(raw) }
}

/**
 * Build a display feed that interleaves optional user turns with stream blocks.
 * User turns are supplied by the chat composer (not inferred from PTY echo).
 */
export type FeedItem =
  | { type: 'user'; id: string; text: string }
  | { type: 'block'; block: StreamBlock }

export function buildChatFeed(
  userTurns: { id: string; text: string }[],
  streamBlocks: StreamBlock[],
): FeedItem[] {
  const items: FeedItem[] = []
  for (const u of userTurns) {
    items.push({ type: 'user', id: u.id, text: u.text })
  }
  for (const b of streamBlocks) {
    items.push({ type: 'block', block: b })
  }
  return items
}
