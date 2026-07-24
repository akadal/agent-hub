/**
 * Tests the shipped stream feed (line editor + bounded blocks).
 * Run: npm test
 */
import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

import {
  FEED_MAX_BLOCKS,
  createStreamFeed,
  feedAddUser,
  feedFlushPartial,
  feedPush,
  feedSnapshot,
  isSlashCommandLine,
  parseStreamToBlocks,
  stripAnsi,
} from './stream-blocks.ts'

function kinds(blocks: ReturnType<typeof feedSnapshot>) {
  return blocks.map((b) => b.kind)
}

describe('stream feed — line editor', () => {
  it('strips ANSI colors to plain text lines', () => {
    const feed = createStreamFeed()
    feedPush(feed, '\u001b[32mhello\u001b[0m world\n')
    const blocks = feedSnapshot(feed)
    assert.equal(blocks.length, 1)
    assert.equal(blocks[0]!.kind, 'output')
    assert.equal(blocks[0]!.text, 'hello world')
    assert.equal(stripAnsi('\u001b[31mred\u001b[0m').includes('\u001b'), false)
  })

  it('CR overwrite keeps only the final progress line', () => {
    const feed = createStreamFeed()
    // spinner / download progress: rewrite same line via CR
    feedPush(feed, 'Downloading 10%\rDownloading 50%\rDownloading 100%\n')
    const blocks = feedSnapshot(feed)
    assert.equal(blocks.length, 1)
    assert.equal(blocks[0]!.text, 'Downloading 100%')
    assert.ok(!blocks[0]!.text.includes('10%'))
  })

  it('erase-line CSI clears partial content', () => {
    const feed = createStreamFeed()
    feedPush(feed, 'old text\u001b[2Kfresh\n')
    const blocks = feedSnapshot(feed)
    assert.equal(blocks[0]!.text, 'fresh')
  })

  it('backspace edits the current line', () => {
    const feed = createStreamFeed()
    feedPush(feed, 'helo\blo\n') // helo → backspace o → hel → lo → hello
    const blocks = feedSnapshot(feed)
    assert.equal(blocks[0]!.text, 'hello')
  })
})

describe('stream feed — classification', () => {
  it('keeps slash commands visible', () => {
    const feed = createStreamFeed()
    feedPush(feed, 'ready\n/help\n/status verbose\ndone\n')
    const blocks = feedSnapshot(feed)
    assert.ok(kinds(blocks).includes('command'))
    const cmds = blocks.filter((b) => b.kind === 'command')
    assert.ok(cmds.some((b) => b.text.includes('/help')))
    assert.ok(cmds.some((b) => b.text.includes('/status verbose')))
    assert.equal(isSlashCommandLine('/usr/local/bin/node'), false)
  })

  it('marks thinking regions collapsible', () => {
    const feed = createStreamFeed()
    feedPush(
      feed,
      'Before\n<thinking>\ncheck the file first\nrun tests\n</thinking>\nAfter answer\n',
    )
    const blocks = feedSnapshot(feed)
    const thinking = blocks.filter((b) => b.kind === 'thinking')
    assert.ok(thinking.length >= 1)
    assert.equal(thinking[0]!.collapsible, true)
    assert.match(thinking.map((b) => b.text).join('\n'), /check the file/)
    assert.ok(blocks.some((b) => b.kind === 'output' && /Before/.test(b.text)))
    assert.ok(
      blocks.some((b) => b.kind === 'output' && /After answer/.test(b.text)),
    )
  })

  it('handles soft thinking: marker', () => {
    const feed = createStreamFeed()
    feedPush(feed, 'thinking: consider edge cases\nHere is the fix.\n')
    // soft thinking stays open until close marker — seal with next non-thinking
    // "Here is the fix" is output; thinking line was inline open
    const blocks = feedSnapshot(feed)
    assert.ok(blocks.some((b) => b.kind === 'thinking'))
    assert.ok(blocks.some((b) => b.kind === 'output' && /fix/.test(b.text)))
  })

  it('merges consecutive output lines into fewer blocks', () => {
    const feed = createStreamFeed()
    for (let i = 0; i < 8; i++) feedPush(feed, `line ${i}\n`)
    const blocks = feedSnapshot(feed)
    // Should be one grouped output block, not 8
    assert.equal(blocks.length, 1)
    assert.equal(blocks[0]!.kind, 'output')
    assert.match(blocks[0]!.text, /line 0/)
    assert.match(blocks[0]!.text, /line 7/)
  })

  it('user turns seal prior output', () => {
    const feed = createStreamFeed()
    feedPush(feed, 'prompt$\n')
    feedAddUser(feed, '/status')
    feedPush(feed, 'ok\n')
    const blocks = feedSnapshot(feed)
    assert.deepEqual(kinds(blocks), ['output', 'user', 'output'])
    assert.equal(blocks[1]!.text, '/status')
  })

  it('caps block count under heavy traffic', () => {
    const feed = createStreamFeed()
    // Force many sealed blocks via alternating command/output
    for (let i = 0; i < FEED_MAX_BLOCKS + 40; i++) {
      feedPush(feed, `/cmd${i}\n`)
      feedPush(feed, `out ${i}\n`)
    }
    const blocks = feedSnapshot(feed)
    assert.ok(blocks.length <= FEED_MAX_BLOCKS + 1) // + open
    assert.ok(blocks.length < FEED_MAX_BLOCKS + 40)
  })

  it('parseStreamToBlocks one-shot matches incremental shape', () => {
    const raw = 'a\nb\n/help\nc\n'
    const blocks = parseStreamToBlocks(raw)
    assert.ok(blocks.some((b) => b.kind === 'command'))
    assert.ok(blocks.some((b) => b.kind === 'output'))
  })

  it('flush partial commits prompt without trailing newline', () => {
    const feed = createStreamFeed()
    feedPush(feed, 'user@host:~$ ')
    assert.equal(feedSnapshot(feed).length, 0)
    feedFlushPartial(feed)
    assert.equal(feedSnapshot(feed)[0]!.text, 'user@host:~$')
  })
})
