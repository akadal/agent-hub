/**
 * Lite SSH stream → readable feed.
 *
 * Raw PTY bytes go through a line editor (CR overwrite, backspace, erase-line,
 * ANSI/CSI drop) then a cleaner that removes prompts, echoes, and consecutive
 * repeats. Output is a capped block list — never a full-buffer re-parse.
 */

export type StreamBlockKind = 'output' | 'thinking' | 'command' | 'system' | 'user'

export interface StreamBlock {
  id: string
  kind: StreamBlockKind
  text: string
  collapsible: boolean
}

/** Caps keep the feed readable and the tab responsive. */
export const FEED_MAX_BLOCKS = 60
export const FEED_MAX_LINES = 300
export const FEED_MAX_CHARS = 48_000
/** Merge consecutive plain output into fewer blocks. */
export const FEED_GROUP_LINES = 24
/** Remember last N user inputs to drop shell echoes. */
const RECENT_USER_MAX = 12

const THINKING_OPEN =
  /^(?:<\s*thinking\s*>|\[\s*thinking\s*\]|thinking\s*:|reasoning\s*:|thought\s*:)$/i
const THINKING_OPEN_INLINE =
  /^(?:thinking|reasoning|thought)\s*:\s+(.+)$/i
const THINKING_CLOSE =
  /^(?:<\/\s*thinking\s*>|\[\s*\/\s*thinking\s*\]|end\s+thinking\b)$/i
const SYSTEM_LINE =
  /^\[(?:connection lost|reconnected|session closed|ssh ready)[^\]]*\]$/i

/**
 * Shell prompt at start of line, e.g.:
 *   32d3aaa375dc:~#
 *   root@host:/var#
 *   user@host:~/src$
 */
const PROMPT_AT_START =
  /^(?:[\w.-]+@)?[\w.-]+:[^\n#$]*[#\$]\s*/

/** Whole line is only a prompt (no command). */
const PROMPT_ONLY =
  /^(?:[\w.-]+@)?[\w.-]+:[^\n#$]*[#\$]\s*$/

export function isSlashCommandLine(line: string): boolean {
  const t = line.trimStart()
  if (!t.startsWith('/')) return false
  if (t.startsWith('//') || t.startsWith('/*')) return false
  if (/^\/[A-Za-z0-9._-]+\//.test(t)) return false
  return /^\/[A-Za-z][\w-]*(?:\s|$)/.test(t)
}

/**
 * Remove shell chrome from a finished line: prompt prefix, PS2 `> `,
 * trailing whitespace. Exported for tests.
 */
export function cleanShellLine(line: string): string {
  let s = line.replace(/\s+$/g, '')
  // Drop OSC-like leftovers that slipped through as printable garbage
  s = s.replace(/\u0000/g, '')
  // Shell prompt prefix (may appear after CR redraw on same visual line)
  s = s.replace(PROMPT_AT_START, '')
  // Secondary / continuation prompt
  if (/^>\s+/.test(s)) s = s.replace(/^>\s+/, '')
  // Some shells echo: "# command" after strip fails — rare
  return s.replace(/\s+$/g, '')
}

export function isPromptOnlyLine(line: string): boolean {
  const t = line.replace(/\s+$/g, '')
  if (!t.trim()) return true
  return PROMPT_ONLY.test(t)
}

/** Strip CSI/OSC/charset for one-shot plain text (tests + system notes). */
export function stripAnsi(input: string): string {
  let out = ''
  let i = 0
  while (i < input.length) {
    const c = input.charCodeAt(i)
    if (c === 0x1b) {
      i = skipEscape(input, i)
      continue
    }
    if (c === 0x0d) {
      out += '\n'
      i++
      continue
    }
    if (c === 0x0a) {
      out += '\n'
      i++
      continue
    }
    if (c < 0x20 && c !== 0x09) {
      i++
      continue
    }
    out += input[i]!
    i++
  }
  return out
}

/** Skip an ESC sequence starting at `i` (points at ESC). Returns new index. */
function skipEscape(s: string, i: number): number {
  if (i >= s.length || s.charCodeAt(i) !== 0x1b) return i + 1
  i++
  if (i >= s.length) return i
  const n = s.charCodeAt(i)

  // CSI: ESC [ params final
  if (n === 0x5b /* [ */) {
    i++
    while (i < s.length) {
      const ch = s.charCodeAt(i)
      i++
      if (ch >= 0x40 && ch <= 0x7e) break
    }
    return i
  }

  // OSC: ESC ] … BEL or ST
  if (n === 0x5d /* ] */) {
    i++
    while (i < s.length) {
      const ch = s.charCodeAt(i)
      if (ch === 0x07) return i + 1
      if (ch === 0x1b && s.charCodeAt(i + 1) === 0x5c) return i + 2
      i++
    }
    return i
  }

  // SS3 / charset designations
  if (n === 0x28 || n === 0x29 || n === 0x2a || n === 0x2b || n === 0x4e || n === 0x4f) {
    return Math.min(i + 2, s.length)
  }
  // ESC = / ESC > application keypad etc.
  if (n === 0x3d || n === 0x3e) return i + 1
  return i + 1
}

// ── Line editor ─────────────────────────────────────────────────────────────

interface LineEditor {
  buf: string
  cursor: number
  esc: string
}

function newEditor(): LineEditor {
  return { buf: '', cursor: 0, esc: '' }
}

function editorReset(ed: LineEditor) {
  ed.buf = ''
  ed.cursor = 0
}

function editorWrite(ed: LineEditor, ch: string) {
  if (ed.cursor >= ed.buf.length) {
    ed.buf += ch
  } else {
    ed.buf = ed.buf.slice(0, ed.cursor) + ch + ed.buf.slice(ed.cursor + 1)
  }
  ed.cursor++
}

function editorBackspace(ed: LineEditor) {
  if (ed.cursor <= 0) return
  ed.cursor--
  ed.buf = ed.buf.slice(0, ed.cursor) + ed.buf.slice(ed.cursor + 1)
}

function editorEraseLine(ed: LineEditor, mode: number) {
  if (mode === 2) {
    editorReset(ed)
    return
  }
  if (mode === 1) {
    ed.buf = ed.buf.slice(ed.cursor)
    ed.cursor = 0
    return
  }
  ed.buf = ed.buf.slice(0, ed.cursor)
}

// ── Feed ────────────────────────────────────────────────────────────────────

export interface StreamFeed {
  ed: LineEditor
  lines: { kind: StreamBlockKind; text: string }[]
  blocks: StreamBlock[]
  open: StreamBlock | null
  openLineCount: number
  inThinking: boolean
  nextId: number
  totalChars: number
  gen: number
  /** Last committed display text (for consecutive dedupe). */
  lastText: string
  /** Recent user composer lines — used to drop local echo. */
  recentUser: string[]
}

export function createStreamFeed(): StreamFeed {
  return {
    ed: newEditor(),
    lines: [],
    blocks: [],
    open: null,
    openLineCount: 0,
    inThinking: false,
    nextId: 1,
    totalChars: 0,
    gen: 0,
    lastText: '',
    recentUser: [],
  }
}

function sealOpen(feed: StreamFeed) {
  if (!feed.open) return
  if (!feed.open.text.trim()) {
    feed.open = null
    feed.openLineCount = 0
    return
  }
  feed.blocks.push(feed.open)
  feed.open = null
  feed.openLineCount = 0
}

function trimFeed(feed: StreamFeed) {
  while (
    feed.blocks.length > FEED_MAX_BLOCKS ||
    feed.totalChars > FEED_MAX_CHARS ||
    feed.lines.length > FEED_MAX_LINES
  ) {
    const dropped = feed.blocks.shift()
    if (!dropped) break
    feed.totalChars -= dropped.text.length
    const n = Math.max(1, dropped.text.split('\n').length)
    feed.lines.splice(0, n)
  }
  if (feed.totalChars < 0) {
    feed.totalChars = feed.blocks.reduce((a, b) => a + b.text.length, 0)
  }
}

/** True when this PTY line is a local echo of a recent composer submit. */
function isUserEcho(feed: StreamFeed, text: string): boolean {
  for (let i = 0; i < feed.recentUser.length; i++) {
    const u = feed.recentUser[i]!
    if (text === u) {
      feed.recentUser.splice(i, 1)
      return true
    }
    // Multi-line submit: drop each echoed physical line
    const parts = u.split(/\r?\n/)
    if (parts.length > 1 && parts.includes(text)) return true
    // Partial first-line echo while the user command is longer
    if (u.startsWith(text) && text.length >= 4) return true
    // Shell re-wrapped a quoted command (printf '…') — drop fragments that
    // still contain quotes/backslashes from the submitted string. Do NOT use
    // bare `%` here: progress output like "Downloading 10%" would false-match
    // against a printf format string containing `%%`.
    if (
      text.length >= 4 &&
      /['"`\\]/.test(text) &&
      u.includes(text)
    ) {
      return true
    }
  }
  return false
}

/**
 * Normalize a raw committed line into display text, or null to drop.
 */
export function normalizeCommittedLine(
  feed: StreamFeed,
  rawLine: string,
): { kind: StreamBlockKind; text: string } | null {
  // Prompt-only (incl. after CSI stripped): "host:~# "
  if (isPromptOnlyLine(rawLine)) return null

  let text = cleanShellLine(rawLine)
  if (!text.trim()) return null

  // Drop pure cursor/status leftovers / dangling quote from PS2 close
  if (/^[\[\]?0-9;]*$/.test(text)) return null
  if (/^['"`\\]+$/.test(text.trim())) return null

  if (isUserEcho(feed, text)) return null

  // printf-style shell echoes keep `%%`; real progress almost never does.
  if (text.includes('%%')) return null

  // Consecutive identical lines (progress spam, repeated echo)
  if (text === feed.lastText) return null

  if (SYSTEM_LINE.test(text.trim())) {
    return { kind: 'system', text: text.trim() }
  }

  if (feed.inThinking) {
    if (THINKING_CLOSE.test(text.trim())) {
      feed.inThinking = false
      return null
    }
    return { kind: 'thinking', text }
  }

  if (THINKING_CLOSE.test(text.trim())) {
    feed.inThinking = false
    return null
  }

  const inline = text.trim().match(THINKING_OPEN_INLINE)
  if (inline) {
    return { kind: 'thinking', text: inline[1] ?? text }
  }

  if (THINKING_OPEN.test(text.trim())) {
    feed.inThinking = true
    return null
  }

  const xmlOpen = text.trim().match(/^<\s*thinking\s*>\s*(.*)$/i)
  if (xmlOpen) {
    feed.inThinking = true
    if (xmlOpen[1]) return { kind: 'thinking', text: xmlOpen[1] }
    return null
  }

  if (isSlashCommandLine(text)) {
    return { kind: 'command', text: text.trim() }
  }

  return { kind: 'output', text }
}

const PROGRESS_LINE = /^(.*?)(\d{1,3})\s*%\s*$/

function pushClassified(
  feed: StreamFeed,
  kind: StreamBlockKind,
  text: string,
) {
  // Collapse "Downloading 10%" → "50%" → "100%" into the last frame only.
  if (kind === 'output' && feed.open?.kind === 'output') {
    const prog = text.match(PROGRESS_LINE)
    if (prog) {
      const lines = feed.open.text.split('\n')
      const prev = lines[lines.length - 1] ?? ''
      const prevProg = prev.match(PROGRESS_LINE)
      if (prevProg && prevProg[1] === prog[1]) {
        const oldLen = prev.length
        lines[lines.length - 1] = text
        feed.open.text = lines.join('\n')
        feed.totalChars += text.length - oldLen
        feed.lastText = text
        feed.gen++
        return
      }
    }
  }

  feed.lines.push({ kind, text })
  feed.lastText = text

  const collapsible = kind === 'thinking'
  const canMerge =
    feed.open &&
    feed.open.kind === kind &&
    (kind === 'output' || kind === 'thinking') &&
    feed.openLineCount < FEED_GROUP_LINES

  if (canMerge && feed.open) {
    feed.open.text += '\n' + text
    feed.openLineCount++
    feed.totalChars += text.length + 1
  } else {
    sealOpen(feed)
    if (kind === 'output' || kind === 'thinking') {
      feed.open = {
        id: `b${feed.nextId++}`,
        kind,
        text,
        collapsible,
      }
      feed.openLineCount = 1
      feed.totalChars += text.length
    } else {
      const block: StreamBlock = {
        id: `b${feed.nextId++}`,
        kind,
        text,
        collapsible: false,
      }
      feed.blocks.push(block)
      feed.totalChars += text.length
    }
  }

  trimFeed(feed)
  feed.gen++
}

function commitEditorLine(feed: StreamFeed) {
  const raw = feed.ed.buf
  editorReset(feed.ed)
  const classified = normalizeCommittedLine(feed, raw)
  if (!classified) return
  pushClassified(feed, classified.kind, classified.text)
}

function applyCsi(ed: LineEditor, params: string, final: string) {
  const n = params === '' ? 0 : parseInt(params.split(';')[0] || '0', 10)
  switch (final) {
    case 'K':
      editorEraseLine(ed, Number.isFinite(n) ? n : 0)
      break
    case 'J':
      // Clear screen / display — only wipe the in-progress line, keep history.
      if (n === 2 || n === 3) editorReset(ed)
      else if (n === 0) editorEraseLine(ed, 0)
      break
    case 'C':
      ed.cursor = Math.min(ed.buf.length, ed.cursor + (n || 1))
      break
    case 'D':
      ed.cursor = Math.max(0, ed.cursor - (n || 1))
      break
    case 'G':
      ed.cursor = Math.max(0, Math.min(ed.buf.length, (n || 1) - 1))
      break
    // n = DSR, A/B/H/f cursor motion — ignore (line-oriented viewer)
    default:
      break
  }
}

/**
 * Push a raw PTY chunk into the feed. Mutates `feed`.
 * Returns true when displayable content changed.
 */
export function feedPush(feed: StreamFeed, chunk: string): boolean {
  if (!chunk) return false
  const gen0 = feed.gen
  let i = 0
  const s = chunk
  const ed = feed.ed

  if (ed.esc) {
    const combined = ed.esc + s
    ed.esc = ''
    return feedPush(feed, combined)
  }

  while (i < s.length) {
    const c = s.charCodeAt(i)

    if (c === 0x1b) {
      if (i + 1 >= s.length) {
        ed.esc = s.slice(i)
        break
      }
      const next = s.charCodeAt(i + 1)
      if (next === 0x5b /* [ */) {
        let j = i + 2
        while (j < s.length) {
          const ch = s.charCodeAt(j)
          if (ch >= 0x40 && ch <= 0x7e) {
            const params = s.slice(i + 2, j)
            applyCsi(ed, params, s[j]!)
            i = j + 1
            break
          }
          j++
        }
        if (j >= s.length) {
          ed.esc = s.slice(i)
          break
        }
        continue
      }
      const after = skipEscape(s, i)
      if (
        next === 0x5d &&
        after >= s.length &&
        !s.includes('\u0007', i) &&
        !s.includes('\u001b\\', i)
      ) {
        ed.esc = s.slice(i)
        break
      }
      i = after
      continue
    }

    if (c === 0x0d) {
      // CR: return to column 0 (progress bars / prompt redraw)
      ed.cursor = 0
      i++
      continue
    }

    if (c === 0x0a) {
      commitEditorLine(feed)
      i++
      continue
    }

    if (c === 0x08 || c === 0x7f) {
      editorBackspace(ed)
      i++
      continue
    }

    if (c === 0x09) {
      const spaces = 8 - (ed.cursor % 8)
      for (let t = 0; t < spaces; t++) editorWrite(ed, ' ')
      i++
      continue
    }

    if (c < 0x20) {
      i++
      continue
    }

    editorWrite(ed, s[i]!)
    i++
  }

  return feed.gen !== gen0
}

export function feedSnapshot(feed: StreamFeed): StreamBlock[] {
  if (!feed.open || !feed.open.text.trim()) return feed.blocks.slice()
  return [...feed.blocks, feed.open]
}

/**
 * Record a user-sent line (composer). Seals current output so new PTY
 * output starts a fresh block after the user bubble. Remembers text so
 * local echo of the same line is dropped.
 */
export function feedAddUser(feed: StreamFeed, text: string): StreamBlock {
  sealOpen(feed)
  const block: StreamBlock = {
    id: `u${feed.nextId++}`,
    kind: 'user',
    text,
    collapsible: false,
  }
  feed.blocks.push(block)
  feed.totalChars += text.length
  feed.lines.push({ kind: 'user', text })
  feed.lastText = '' // allow shell to print same text as result
  feed.recentUser.push(text)
  if (feed.recentUser.length > RECENT_USER_MAX) {
    feed.recentUser.splice(0, feed.recentUser.length - RECENT_USER_MAX)
  }
  trimFeed(feed)
  feed.gen++
  return block
}

/** Flush incomplete line (prompt sitting without NL). */
export function feedFlushPartial(feed: StreamFeed): boolean {
  if (!feed.ed.buf.trim()) return false
  // Bare prompt without NL — drop rather than show "host:~#"
  if (isPromptOnlyLine(feed.ed.buf) || !cleanShellLine(feed.ed.buf).trim()) {
    editorReset(feed.ed)
    return false
  }
  commitEditorLine(feed)
  return true
}

export function parseStreamToBlocks(raw: string): StreamBlock[] {
  const feed = createStreamFeed()
  feedPush(feed, raw.endsWith('\n') ? raw : raw + '\n')
  feedFlushPartial(feed)
  return feedSnapshot(feed).filter((b) => b.kind !== 'user')
}

export function appendStreamChunk(
  prevRaw: string,
  chunk: string,
): { raw: string; blocks: StreamBlock[] } {
  const raw = prevRaw + chunk
  return { raw, blocks: parseStreamToBlocks(raw) }
}

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
    if (b.kind === 'user') {
      items.push({ type: 'user', id: b.id, text: b.text })
    } else {
      items.push({ type: 'block', block: b })
    }
  }
  return items
}
