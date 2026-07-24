/**
 * Incremental terminal → chat feed.
 *
 * Raw PTY bytes are reduced through a small line editor (CR overwrite,
 * backspace, erase-line, ANSI strip) into clean logical lines, then grouped
 * into a bounded block list. Never re-parses the full history.
 *
 * Performance:
 * - O(chunk) push, no full-buffer re-parse
 * - hard caps on lines / blocks / characters
 * - UI should throttle setState; this module is pure + mutable state object
 */

export type StreamBlockKind = 'output' | 'thinking' | 'command' | 'system' | 'user'

export interface StreamBlock {
  id: string
  kind: StreamBlockKind
  text: string
  collapsible: boolean
}

/** Caps keep the feed readable and the tab responsive. */
export const FEED_MAX_BLOCKS = 80
export const FEED_MAX_LINES = 400
export const FEED_MAX_CHARS = 80_000
/** Merge consecutive plain output into fewer bubbles. */
export const FEED_GROUP_LINES = 20

const THINKING_OPEN =
  /^(?:<\s*thinking\s*>|\[\s*thinking\s*\]|thinking\s*:|reasoning\s*:|thought\s*:)$/i
const THINKING_OPEN_INLINE =
  /^(?:thinking|reasoning|thought)\s*:\s+(.+)$/i
const THINKING_CLOSE =
  /^(?:<\/\s*thinking\s*>|\[\s*\/\s*thinking\s*\]|end\s+thinking\b)$/i
const SYSTEM_LINE =
  /^\[(?:connection lost|reconnected|session closed|ssh ready)[^\]]*\]$/i

export function isSlashCommandLine(line: string): boolean {
  const t = line.trimStart()
  if (!t.startsWith('/')) return false
  if (t.startsWith('//') || t.startsWith('/*')) return false
  if (/^\/[A-Za-z0-9._-]+\//.test(t)) return false
  return /^\/[A-Za-z][\w-]*(?:\s|$)/.test(t)
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
      // bare CR → treat as newline boundary for one-shot strip
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
  i++ // after ESC
  if (i >= s.length) return i
  const n = s.charCodeAt(i)

  // CSI: ESC [ ... final byte @-~
  if (n === 0x5b /* [ */) {
    i++
    while (i < s.length) {
      const ch = s.charCodeAt(i)
      i++
      if (ch >= 0x40 && ch <= 0x7e) break
    }
    return i
  }

  // OSC: ESC ] ... BEL or ST (ESC \)
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

  // Charset / simple two-byte: ESC ( B, ESC =, etc.
  if (n === 0x28 || n === 0x29 || n === 0x2a || n === 0x2b) {
    return Math.min(i + 2, s.length)
  }
  return i + 1
}

// ── Line editor (mutable, for CR overwrite / progress bars) ─────────────────

interface LineEditor {
  /** Committed cells of the current line (string built with overwrite). */
  buf: string
  cursor: number
  /** Incomplete ESC sequence held across chunks. */
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
    ed.buf =
      ed.buf.slice(0, ed.cursor) + ch + ed.buf.slice(ed.cursor + 1)
  }
  ed.cursor++
}

function editorBackspace(ed: LineEditor) {
  if (ed.cursor <= 0) return
  ed.cursor--
  ed.buf = ed.buf.slice(0, ed.cursor) + ed.buf.slice(ed.cursor + 1)
}

function editorEraseLine(ed: LineEditor, mode: number) {
  // 0: cursor→end, 1: start→cursor, 2: whole line
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

// ── Feed state ──────────────────────────────────────────────────────────────

export interface StreamFeed {
  ed: LineEditor
  /** Ring of classified logical lines (for rebuild if needed). */
  lines: { kind: StreamBlockKind; text: string }[]
  blocks: StreamBlock[]
  /** Open block that still accepts merges. */
  open: StreamBlock | null
  openLineCount: number
  inThinking: boolean
  nextId: number
  totalChars: number
  /** Generation bumps when display content changes. */
  gen: number
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
    // Drop matching prefix lines (approx: drop one line group)
    const n = Math.max(1, dropped.text.split('\n').length)
    feed.lines.splice(0, n)
  }
  // Keep totalChars non-negative
  if (feed.totalChars < 0) {
    feed.totalChars = feed.blocks.reduce((a, b) => a + b.text.length, 0)
  }
}

function classifyLine(
  feed: StreamFeed,
  rawLine: string,
): { kind: StreamBlockKind; text: string } | null {
  // Strip trailing spaces from line-editor buffer; keep leading (prompts).
  const line = rawLine.replace(/\s+$/g, '')
  const trimmed = line.trim()
  if (!trimmed) return null // drop empty lines — they only create noise

  if (SYSTEM_LINE.test(trimmed)) {
    return { kind: 'system', text: trimmed }
  }

  if (feed.inThinking) {
    if (THINKING_CLOSE.test(trimmed)) {
      feed.inThinking = false
      return null
    }
    return { kind: 'thinking', text: line }
  }

  if (THINKING_CLOSE.test(trimmed)) {
    feed.inThinking = false
    return null
  }

  // Single-line "thinking: …" — do not keep the region open (avoids swallowing
  // the next normal reply line).
  const inline = trimmed.match(THINKING_OPEN_INLINE)
  if (inline) {
    return { kind: 'thinking', text: inline[1] ?? line }
  }

  if (THINKING_OPEN.test(trimmed)) {
    feed.inThinking = true
    return null // marker-only line; following lines are thinking until close
  }

  // XML-style open with body on same line: <thinking>foo
  const xmlOpen = trimmed.match(/^<\s*thinking\s*>\s*(.*)$/i)
  if (xmlOpen) {
    feed.inThinking = true
    if (xmlOpen[1]) return { kind: 'thinking', text: xmlOpen[1] }
    return null
  }

  if (isSlashCommandLine(line)) {
    return { kind: 'command', text: trimmed }
  }

  return { kind: 'output', text: line }
}

function pushClassified(
  feed: StreamFeed,
  kind: StreamBlockKind,
  text: string,
) {
  feed.lines.push({ kind, text })

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
      // command / system / user — sealed immediately as their own block
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
  const classified = classifyLine(feed, raw)
  if (!classified) return
  pushClassified(feed, classified.kind, classified.text)
}

/**
 * Apply CSI that affects the line editor. Unknown sequences are ignored.
 * `params` is the numeric parameter string (e.g. "2", "0", "").
 */
function applyCsi(ed: LineEditor, params: string, final: string) {
  const n = params === '' ? 0 : parseInt(params.split(';')[0] || '0', 10)
  switch (final) {
    case 'K': // erase in line
      editorEraseLine(ed, Number.isFinite(n) ? n : 0)
      break
    case 'C': // cursor forward — soft: move cursor without writing
      ed.cursor = Math.min(ed.buf.length, ed.cursor + (n || 1))
      break
    case 'D': // cursor back
      ed.cursor = Math.max(0, ed.cursor - (n || 1))
      break
    case 'G': // absolute column (1-based)
      ed.cursor = Math.max(0, Math.min(ed.buf.length, (n || 1) - 1))
      break
    // A/B/H/f/J screen motions: ignore (we are a line feed, not a full TTY)
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

  // Resume incomplete escape held from previous chunk
  if (ed.esc) {
    const combined = ed.esc + s
    ed.esc = ''
    return feedPush(feed, combined)
  }

  while (i < s.length) {
    const c = s.charCodeAt(i)

    if (c === 0x1b) {
      // Need at least ESC + one more; if incomplete, stash
      if (i + 1 >= s.length) {
        ed.esc = s.slice(i)
        break
      }
      const next = s.charCodeAt(i + 1)
      if (next === 0x5b /* [ */) {
        // CSI — find final byte
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
      // OSC or other — skip whole sequence (may span chunk)
      const after = skipEscape(s, i)
      if (after === i + 1 && i + 1 >= s.length) {
        ed.esc = s.slice(i)
        break
      }
      // If skipEscape stopped at end mid-OSC, stash remainder
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
      // CR: return to start of line (progress bar / spinner overwrite)
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
      // tab → spaces to next 8-col
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

/** Snapshot of sealed + open blocks for rendering. */
export function feedSnapshot(feed: StreamFeed): StreamBlock[] {
  if (!feed.open || !feed.open.text.trim()) return feed.blocks.slice()
  return [...feed.blocks, feed.open]
}

/**
 * Record a user-sent line (composer). Seals current output so new PTY
 * output starts a fresh block after the user bubble.
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
  trimFeed(feed)
  feed.gen++
  return block
}

/** Flush incomplete line (e.g. before snapshot if prompt sits without NL). */
export function feedFlushPartial(feed: StreamFeed): boolean {
  if (!feed.ed.buf.trim()) return false
  commitEditorLine(feed)
  return true
}

// ── Compatibility helpers (tests + simple one-shot parse) ───────────────────

/**
 * One-shot parse of a complete stream into blocks (uses the incremental feed).
 * Prefer `feedPush` for live I/O.
 */
export function parseStreamToBlocks(raw: string): StreamBlock[] {
  const feed = createStreamFeed()
  feedPush(feed, raw.endsWith('\n') ? raw : raw + '\n')
  feedFlushPartial(feed)
  return feedSnapshot(feed).filter((b) => b.kind !== 'user')
}

/**
 * @deprecated Prefer createStreamFeed + feedPush. Kept for call sites that
 * still pass full buffers — still uses incremental feed under the hood but
 * allocates a new feed each time (do not use in hot path).
 */
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
