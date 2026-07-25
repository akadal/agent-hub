import type { ITheme } from '@xterm/xterm'

import type { ResolvedTheme } from '@/lib/theme-pref'

/**
 * xterm palettes that match the app surface.
 *
 * The light palette is not the dark one with two colours swapped: the default
 * ANSI set is tuned for a black background, and bright yellow/cyan/white on
 * white is unreadable. These are the darkened variants, so `ls`, `git diff`
 * and prompt colours stay legible either way.
 */

const DARK: ITheme = {
  background: '#0a0a0a',
  foreground: '#e5e5e5',
  cursor: '#e5e5e5',
  cursorAccent: '#0a0a0a',
  selectionBackground: '#264f78',
  black: '#1c1c1c',
  red: '#f87171',
  green: '#4ade80',
  yellow: '#fbbf24',
  blue: '#60a5fa',
  magenta: '#c084fc',
  cyan: '#22d3ee',
  white: '#d4d4d4',
  brightBlack: '#6b7280',
  brightRed: '#fca5a5',
  brightGreen: '#86efac',
  brightYellow: '#fde047',
  brightBlue: '#93c5fd',
  brightMagenta: '#d8b4fe',
  brightCyan: '#67e8f9',
  brightWhite: '#fafafa',
}

const LIGHT: ITheme = {
  background: '#ffffff',
  foreground: '#1f2328',
  cursor: '#1f2328',
  cursorAccent: '#ffffff',
  selectionBackground: '#b6d7ff',
  black: '#24292f',
  red: '#cf222e',
  // Green is the one colour that must work as a *background* too: tmux paints
  // its status bar green with black text, and a dark green there is unreadable.
  green: '#2da44e',
  yellow: '#8a6100',
  blue: '#0969da',
  magenta: '#8250df',
  cyan: '#1b7c83',
  white: '#6e7781',
  brightBlack: '#57606a',
  brightRed: '#a40e26',
  brightGreen: '#3fb950',
  brightYellow: '#7d4e00',
  brightBlue: '#0550ae',
  brightMagenta: '#6639ba',
  brightCyan: '#1b7c83',
  brightWhite: '#24292f',
}

export function xtermTheme(theme: ResolvedTheme): ITheme {
  return theme === 'dark' ? DARK : LIGHT
}

/** Backdrop behind the xterm canvas, so resizes never flash the other theme. */
export function terminalSurface(theme: ResolvedTheme): string {
  return theme === 'dark' ? '#0a0a0a' : '#ffffff'
}
