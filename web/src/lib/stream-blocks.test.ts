/**
 * Tests the shipped stream→blocks transform (no re-implemented oracle).
 * Run: npx tsx --test src/lib/stream-blocks.test.ts
 */
import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

import {
  appendStreamChunk,
  parseStreamToBlocks,
  stripAnsi,
  type StreamBlock,
} from './stream-blocks.ts'

function kinds(blocks: StreamBlock[]) {
  return blocks.map((b) => b.kind)
}

function texts(blocks: StreamBlock[]) {
  return blocks.map((b) => b.text)
}

describe('stream → history blocks', () => {
  it('strips ANSI and forms plain output blocks', () => {
    const raw =
      '\u001b[32mhello\u001b[0m world\r\nline2\n'
    assert.equal(stripAnsi(raw).includes('\u001b'), false)
    const blocks = parseStreamToBlocks(raw)
    assert.ok(blocks.length >= 1)
    assert.equal(blocks[0]!.kind, 'output')
    assert.match(blocks[0]!.text, /hello world/)
    assert.match(blocks[0]!.text, /line2/)
    assert.equal(blocks[0]!.collapsible, false)
  })

  it('keeps slash-command lines visible as command blocks', () => {
    const raw = [
      'ready',
      '/help',
      '/status verbose',
      'done',
      '',
    ].join('\n')
    const blocks = parseStreamToBlocks(raw)
    assert.ok(kinds(blocks).includes('command'))
    const cmds = blocks.filter((b) => b.kind === 'command')
    assert.ok(cmds.some((b) => b.text.includes('/help')))
    assert.ok(cmds.some((b) => b.text.includes('/status')))
    // Full text retained (not dropped)
    assert.ok(texts(blocks).join('\n').includes('/help'))
    assert.ok(texts(blocks).join('\n').includes('/status verbose'))
  })

  it('marks multi-line thinking regions collapsible', () => {
    const raw = [
      'Before',
      '<thinking>',
      'I should check the file first.',
      'Then run the tests.',
      '</thinking>',
      'After answer',
      '',
    ].join('\n')
    const blocks = parseStreamToBlocks(raw)
    const thinking = blocks.filter((b) => b.kind === 'thinking')
    assert.equal(thinking.length, 1)
    assert.equal(thinking[0]!.collapsible, true)
    assert.match(thinking[0]!.text, /check the file/)
    assert.match(thinking[0]!.text, /run the tests/)
    // Surrounding output still present
    assert.ok(blocks.some((b) => b.kind === 'output' && /Before/.test(b.text)))
    assert.ok(blocks.some((b) => b.kind === 'output' && /After answer/.test(b.text)))
  })

  it('handles soft thinking: / reasoning: markers', () => {
    const raw = [
      'thinking: consider edge cases',
      '',
      'Here is the fix.',
      '',
    ].join('\n')
    const blocks = parseStreamToBlocks(raw)
    const thinking = blocks.find((b) => b.kind === 'thinking')
    assert.ok(thinking)
    assert.equal(thinking!.collapsible, true)
    assert.match(thinking!.text, /edge cases/)
    assert.ok(blocks.some((b) => b.kind === 'output' && /Here is the fix/.test(b.text)))
  })

  it('appendStreamChunk builds a history-like sequence incrementally', () => {
    let state = appendStreamChunk('', 'user@host$ ls\n')
    state = appendStreamChunk(state.raw, 'a.txt\nb.txt\n')
    state = appendStreamChunk(state.raw, '/status\n')
    state = appendStreamChunk(
      state.raw,
      '[thinking]\nplanning next step\n[/thinking]\nok\n',
    )

    assert.ok(state.blocks.length >= 2)
    assert.ok(kinds(state.blocks).includes('command'))
    assert.ok(kinds(state.blocks).includes('thinking'))
    const th = state.blocks.find((b) => b.kind === 'thinking')
    assert.ok(th?.collapsible)
    assert.match(th!.text, /planning next step/)
    assert.ok(texts(state.blocks).join('\n').includes('/status'))
  })

  it('does not treat absolute paths as slash commands', () => {
    const blocks = parseStreamToBlocks('/usr/local/bin/node --version\n')
    assert.ok(!kinds(blocks).includes('command'))
  })
})
