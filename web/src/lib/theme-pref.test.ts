/**
 * Tests the shipped appearance-preference path (same key/API the UI uses).
 * Run: npx tsx --test src/lib/theme-pref.test.ts
 */
import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

import {
  THEME_PREF_KEY,
  applyTheme,
  getThemePreference,
  isThemePreference,
  resolveTheme,
  setThemePreference,
  type StorageLike,
} from './theme-pref.ts'

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

describe('theme preference', () => {
  it('defaults to system when unset', () => {
    assert.equal(getThemePreference(memStorage()), 'system')
  })

  it('ignores a value written by an older or broken build', () => {
    assert.equal(
      getThemePreference(memStorage({ [THEME_PREF_KEY]: 'midnight' })),
      'system',
    )
  })

  it('round-trips each preference through the same key', () => {
    for (const pref of ['light', 'dark', 'system'] as const) {
      const s = memStorage()
      setThemePreference(pref, s)
      assert.equal(s.data[THEME_PREF_KEY], pref)
      assert.equal(getThemePreference(s), pref)
    }
  })

  it('rejects an unknown preference instead of persisting it', () => {
    const s = memStorage()
    assert.throws(
      () => setThemePreference('neon' as never, s),
      /Invalid theme preference/,
    )
    assert.equal(s.data[THEME_PREF_KEY], undefined)
  })

  it('survives storage that throws on read (Safari private mode)', () => {
    const hostile: StorageLike = {
      getItem() {
        throw new Error('access denied')
      },
      setItem() {
        /* unused */
      },
    }
    assert.equal(getThemePreference(hostile), 'system')
  })

  it('recognises only the three shipped preferences', () => {
    assert.equal(isThemePreference('light'), true)
    assert.equal(isThemePreference('dark'), true)
    assert.equal(isThemePreference('system'), true)
    assert.equal(isThemePreference('auto'), false)
    assert.equal(isThemePreference(null), false)
  })
})

describe('resolveTheme', () => {
  it('follows the OS only when the preference is system', () => {
    assert.equal(resolveTheme('system', true), 'dark')
    assert.equal(resolveTheme('system', false), 'light')
  })

  it('keeps an explicit choice even when the OS disagrees', () => {
    assert.equal(resolveTheme('light', true), 'light')
    assert.equal(resolveTheme('dark', false), 'dark')
  })
})

describe('applyTheme', () => {
  function fakeRoot() {
    const classes = new Set<string>()
    return {
      classes,
      root: {
        classList: {
          toggle(name: string, on: boolean) {
            if (on) classes.add(name)
            else classes.delete(name)
            return on
          },
        } as unknown as DOMTokenList,
        style: { colorScheme: '' } as CSSStyleDeclaration,
      },
    }
  }

  it('adds .dark and sets color-scheme for dark', () => {
    const { classes, root } = fakeRoot()
    applyTheme('dark', root)
    assert.equal(classes.has('dark'), true)
    assert.equal(root.style.colorScheme, 'dark')
  })

  it('removes .dark again when switching back to light', () => {
    const { classes, root } = fakeRoot()
    applyTheme('dark', root)
    applyTheme('light', root)
    assert.equal(classes.has('dark'), false)
    assert.equal(root.style.colorScheme, 'light')
  })

  it('is a no-op without a document', () => {
    assert.doesNotThrow(() => applyTheme('dark', null))
  })
})
