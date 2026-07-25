/**
 * Run: npx tsx --test src/lib/terminal-keys.test.ts
 */
import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

import { controlSequence } from './terminal-keys.ts'

describe('controlSequence', () => {
  it('produces the control codes people actually reach for', () => {
    assert.equal(controlSequence('c'), '\x03') // interrupt
    assert.equal(controlSequence('d'), '\x04') // end of input
    assert.equal(controlSequence('z'), '\x1a') // suspend
    assert.equal(controlSequence('l'), '\x0c') // clear
    assert.equal(controlSequence('r'), '\x12') // history search
  })

  it('treats upper and lower case the same, as a terminal does', () => {
    assert.equal(controlSequence('C'), controlSequence('c'))
    assert.equal(controlSequence('A'), '\x01')
  })

  it('covers the punctuation controls', () => {
    assert.equal(controlSequence('['), '\x1b') // Escape
    assert.equal(controlSequence('\\'), '\x1c')
    assert.equal(controlSequence('@'), '\x00')
    assert.equal(controlSequence(' '), '\x00')
    assert.equal(controlSequence('?'), '\x7f') // DEL
  })

  it('returns null when there is no control form, so the key is sent as typed', () => {
    assert.equal(controlSequence('1'), null)
    assert.equal(controlSequence('ü'), null)
    assert.equal(controlSequence(''), null)
    // Multi-byte input (paste, an escape sequence) is never a single keypress.
    assert.equal(controlSequence('\x1b[A'), null)
    assert.equal(controlSequence('ab'), null)
  })
})
