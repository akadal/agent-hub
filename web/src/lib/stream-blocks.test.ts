/**
 * Tests the shipped stream feed against synthetic + real SSH fixtures.
 * Run: npm test
 */
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, it } from 'node:test'

import {
  FEED_MAX_BLOCKS,
  MAX_PENDING_ESC,
  MAX_PENDING_LINE,
  cleanShellLine,
  createStreamFeed,
  feedAddUser,
  feedFlushPartial,
  feedPush,
  feedSnapshot,
  isPromptOnlyLine,
  isSlashCommandLine,
  parseStreamToBlocks,
  stripAnsi,
} from './stream-blocks.ts'

const here = dirname(fileURLToPath(import.meta.url))
const fixturePath = join(here, 'fixtures/ssh-ws-sample.raw.txt')

function kinds(blocks: ReturnType<typeof feedSnapshot>) {
  return blocks.map((b) => b.kind)
}

function allText(blocks: ReturnType<typeof feedSnapshot>) {
  return blocks.map((b) => b.text).join('\n---\n')
}

describe('stream feed — line editor', () => {
  it('strips ANSI colors to plain text lines', () => {
    const feed = createStreamFeed()
    feedPush(feed, '\u001b[32mhello\u001b[0m world\n')
    const blocks = feedSnapshot(feed)
    assert.equal(blocks.length, 1)
    assert.equal(blocks[0]!.text, 'hello world')
    assert.equal(stripAnsi('\u001b[31mred\u001b[0m').includes('\u001b'), false)
  })

  it('CR overwrite keeps only the final progress line', () => {
    const feed = createStreamFeed()
    feedPush(feed, 'Downloading 10%\rDownloading 50%\rDownloading 100%\n')
    const blocks = feedSnapshot(feed)
    assert.equal(blocks.length, 1)
    assert.equal(blocks[0]!.text, 'Downloading 100%')
  })

  it('erase-line CSI clears partial content', () => {
    const feed = createStreamFeed()
    feedPush(feed, 'old text\u001b[2Kfresh\n')
    assert.equal(feedSnapshot(feed)[0]!.text, 'fresh')
  })

  it('backspace edits the current line', () => {
    const feed = createStreamFeed()
    feedPush(feed, 'helo\blo\n')
    assert.equal(feedSnapshot(feed)[0]!.text, 'hello')
  })

  it('drops DSR CSI 6n (cursor query) noise', () => {
    const feed = createStreamFeed()
    feedPush(feed, '32d3aaa375dc:~# \u001b[6nuname -a\n')
    // prompt stripped, command remains once
    assert.equal(feedSnapshot(feed)[0]!.text, 'uname -a')
  })
})

describe('stream feed — shell chrome cleanup', () => {
  it('cleanShellLine strips alpine / bash prompts', () => {
    assert.equal(cleanShellLine('32d3aaa375dc:~# uname -a'), 'uname -a')
    assert.equal(cleanShellLine('root@box:/var# ls'), 'ls')
    assert.equal(cleanShellLine('user@host:~/src$ go test'), 'go test')
    assert.equal(cleanShellLine('> continuation'), 'continuation')
  })

  it('drops prompt-only lines', () => {
    assert.equal(isPromptOnlyLine('32d3aaa375dc:~# '), true)
    assert.equal(isPromptOnlyLine('32d3aaa375dc:~# uname'), false)
    const feed = createStreamFeed()
    feedPush(feed, '32d3aaa375dc:~# \nLinux ok\n32d3aaa375dc:~# \n')
    const t = allText(feedSnapshot(feed))
    assert.match(t, /Linux ok/)
    assert.ok(!t.includes('32d3aaa375dc'))
  })

  it('dedupes consecutive identical lines', () => {
    const feed = createStreamFeed()
    feedPush(feed, 'same\nsame\nsame\nother\n')
    const blocks = feedSnapshot(feed)
    assert.equal(blocks[0]!.text, 'same\nother')
  })

  it('drops local echo of user composer input', () => {
    const feed = createStreamFeed()
    feedAddUser(feed, 'uname -a')
    feedPush(feed, '32d3aaa375dc:~# uname -a\n')
    feedPush(
      feed,
      'Linux 32d3aaa375dc 6.12.54-linuxkit aarch64 Linux\n',
    )
    const blocks = feedSnapshot(feed)
    // user bubble + output only — no echoed "uname -a"
    assert.equal(kinds(blocks)[0], 'user')
    assert.equal(blocks[0]!.text, 'uname -a')
    const out = blocks.filter((b) => b.kind === 'output').map((b) => b.text).join('\n')
    assert.match(out, /Linux /)
    assert.ok(!out.includes('uname -a'))
  })

  it('keeps slash commands visible', () => {
    const feed = createStreamFeed()
    feedPush(feed, 'ready\n/help\n/status verbose\ndone\n')
    assert.ok(kinds(feedSnapshot(feed)).includes('command'))
    assert.equal(isSlashCommandLine('/usr/local/bin/node'), false)
  })

  it('marks thinking regions collapsible', () => {
    const feed = createStreamFeed()
    feedPush(
      feed,
      'Before\n<thinking>\ncheck the file first\n</thinking>\nAfter answer\n',
    )
    const thinking = feedSnapshot(feed).filter((b) => b.kind === 'thinking')
    assert.ok(thinking.length >= 1)
    assert.equal(thinking[0]!.collapsible, true)
  })

  it('caps block count under heavy traffic', () => {
    const feed = createStreamFeed()
    for (let i = 0; i < FEED_MAX_BLOCKS + 40; i++) {
      feedPush(feed, `/cmd${i}\n`)
      feedPush(feed, `out ${i}\n`)
    }
    assert.ok(feedSnapshot(feed).length <= FEED_MAX_BLOCKS + 1)
  })

  it('flush partial drops bare prompt without NL', () => {
    const feed = createStreamFeed()
    feedPush(feed, '32d3aaa375dc:~# ')
    assert.equal(feedFlushPartial(feed), false)
    assert.equal(feedSnapshot(feed).length, 0)
  })
})

describe('stream feed — real agent-hub WS fixture', () => {
  it('cleans dummy docker SSH session into lite readable lines', () => {
    const raw = readFileSync(fixturePath, 'utf8')
    assert.ok(raw.length > 100, 'fixture present')

    const feed = createStreamFeed()
    // Simulate chat composer sends (order matches capture script)
    for (const cmd of [
      'uname -a',
      'ls --color=always /',
      "printf 'Downloading 10%%\\rDownloading 50%%\\rDownloading 100%%\\n'",
      'echo /help',
      'for i in 1 2 3; do echo line$i; done',
      'echo same\necho same\necho same',
    ]) {
      feedAddUser(feed, cmd)
    }
    feedPush(feed, raw)
    feedFlushPartial(feed)

    const blocks = feedSnapshot(feed)
    const text = allText(blocks)

    // No raw CSI / DSR left
    assert.ok(!text.includes('\u001b'), 'no ESC in display text')
    assert.ok(!text.includes('[6n'))

    // No host prompt chrome
    assert.ok(!/32d3aaa375dc:~#/.test(text), `prompt leaked:\n${text}`)

    // Real content kept
    assert.match(text, /Linux /)
    assert.match(text, /\bbin\b/)
    assert.match(text, /line1/)
    assert.match(text, /\/help/)

    // MOTD readable once
    assert.match(text, /Welcome to Alpine/)

    // Consecutive "same" not tripled as three output lines after dedupe
    const sameCount = (text.match(/^same$/gm) || []).length
    assert.ok(sameCount <= 1, `same repeated ${sameCount} times:\n${text}`)

    // Progress frames collapsed to final percentage (ignore user-bubble text)
    const outOnly = blocks
      .filter((b) => b.kind === 'output')
      .map((b) => b.text)
      .join('\n')
    const dl = outOnly.match(/^Downloading \d+%$/gm) || []
    assert.deepEqual(dl, ['Downloading 100%'], `progress frames:\n${outOnly}`)

    // Bounded
    assert.ok(blocks.length < 40, `too many blocks: ${blocks.length}`)
    assert.ok(blocks.length > 0)
  })

  it('parseStreamToBlocks one-shot still works', () => {
    const blocks = parseStreamToBlocks('a\nb\n/help\nc\n')
    assert.ok(blocks.some((b) => b.kind === 'command'))
  })
})

describe('unbounded input (the freeze that disabled stream view)', () => {
  it('commits a runaway no-newline line instead of growing one string', () => {
    const feed = createStreamFeed()
    // 200 KB with no newline at all — `cat` on a binary, or a CR-only spinner.
    for (let i = 0; i < 200; i++) feedPush(feed, 'x'.repeat(1024))
    assert.ok(
      feed.ed.buf.length <= MAX_PENDING_LINE,
      `pending line grew to ${feed.ed.buf.length}`,
    )
    // Still bounded overall — the cap must not defeat the block trimmer.
    assert.ok(feed.blocks.length <= FEED_MAX_BLOCKS)
  })

  it('drops a truncated escape sequence rather than carrying it forever', () => {
    const feed = createStreamFeed()
    // An ESC that never terminates: each chunk used to be prepended to the
    // next and the whole thing re-parsed from the start.
    feedPush(feed, '\x1b[')
    for (let i = 0; i < 40; i++) feedPush(feed, '1'.repeat(64))
    assert.ok(
      feed.ed.esc.length <= MAX_PENDING_ESC,
      `pending escape grew to ${feed.ed.esc.length}`,
    )
  })

  it('still parses a normal split escape sequence across chunks', () => {
    const feed = createStreamFeed()
    feedPush(feed, 'red \x1b')
    feedPush(feed, '[31mtext\x1b[0m done\n')
    const text = feedSnapshot(feed)
      .map((b) => b.text)
      .join('\n')
    assert.equal(text, 'red text done')
  })

  it('finishes a large chunk in reasonable time', () => {
    const feed = createStreamFeed()
    const chunk = 'a'.repeat(64 * 1024)
    const started = process.hrtime.bigint()
    for (let i = 0; i < 16; i++) feedPush(feed, chunk)
    const ms = Number(process.hrtime.bigint() - started) / 1e6
    // Quadratic behaviour on 1 MB took many seconds; linear is milliseconds.
    assert.ok(ms < 2000, `1 MB of no-newline output took ${ms.toFixed(0)}ms`)
  })
})

describe('tmux status bar', () => {
  it('is dropped so the clock does not reappear every minute', () => {
    const feed = createStreamFeed()
    feedPush(feed, 'real output\n')
    feedPush(feed, '[ah-8a66040:sh*    "host" 21:53 25-Jul-26\n')
    feedPush(feed, '[ah-8a66040:sh*    "host" 21:54 25-Jul-26\n')
    const text = feedSnapshot(feed)
      .map((b) => b.text)
      .join('\n')
    assert.equal(text, 'real output')
  })

  it('keeps a user line that merely starts with a bracket', () => {
    const feed = createStreamFeed()
    feedPush(feed, '[warn] disk almost full\n')
    const text = feedSnapshot(feed)
      .map((b) => b.text)
      .join('\n')
    assert.equal(text, '[warn] disk almost full')
  })
})
