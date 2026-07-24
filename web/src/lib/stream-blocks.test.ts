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
