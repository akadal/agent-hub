/**
 * Tests the shipped get/set preference path (same key/API the UI uses).
 * Run: npx tsx --test src/lib/terminal-view-pref.test.ts
 */
import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

import {
  TERMINAL_VIEW_PREF_KEY,
  getTerminalViewMode,
  setTerminalViewMode,
  toggleTerminalViewMode,
  type StorageLike,
} from './terminal-view-pref.ts'

function memStorage(initial: Record<string, string> = {}): StorageLike & {
  data: Record<string, string>
} {
  const data = { ...initial }
  return {
    data,
    getItem(k) {
      return Object.prototype.hasOwnProperty.call(data, k) ? data[k]! : null
    },
    setItem(k, v) {
      data[k] = String(v)
    },
  }
}

describe('terminal view preference (global)', () => {
  it('defaults to classic when unset', () => {
    const s = memStorage()
    assert.equal(getTerminalViewMode(s), 'classic')
  })

  it('set chat → read back chat via same key', () => {
    const s = memStorage()
    setTerminalViewMode('chat', s)
    assert.equal(s.data[TERMINAL_VIEW_PREF_KEY], 'chat')
    assert.equal(getTerminalViewMode(s), 'chat')
  })

  it('set classic → read back classic', () => {
    const s = memStorage()
    setTerminalViewMode('chat', s)
    setTerminalViewMode('classic', s)
    assert.equal(getTerminalViewMode(s), 'classic')
  })

  it('simulates a second session load: last write wins, global', () => {
    // Shared storage = shared preference across “sessions” / reloads.
    const shared = memStorage()
    setTerminalViewMode('chat', shared)

    // Session A load
    const modeA = getTerminalViewMode(shared)
    assert.equal(modeA, 'chat')

    // Session B load sees same global value
    const modeB = getTerminalViewMode(shared)
    assert.equal(modeB, 'chat')

    // Session B toggles to classic
    setTerminalViewMode('classic', shared)

    // Session A re-reads and sees classic (global, not per-session)
    assert.equal(getTerminalViewMode(shared), 'classic')
  })

  it('toggle flips and persists', () => {
    const s = memStorage()
    assert.equal(toggleTerminalViewMode(s), 'chat')
    assert.equal(getTerminalViewMode(s), 'chat')
    assert.equal(toggleTerminalViewMode(s), 'classic')
    assert.equal(getTerminalViewMode(s), 'classic')
  })

  it('invalid stored value falls back to classic', () => {
    const s = memStorage({ [TERMINAL_VIEW_PREF_KEY]: 'neon-crt' })
    assert.equal(getTerminalViewMode(s), 'classic')
  })
})
