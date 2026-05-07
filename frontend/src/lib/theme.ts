// Theme management for Prism.
//
// Why a dedicated module: the previous implementation lived inside the
// SettingsPage component, so the persisted theme was only applied AFTER the
// user navigated to Settings. On every cold start the app rendered with the
// hard-coded `<html class="dark">` from index.html, then "snapped" to light
// (or whatever was in localStorage) the moment SettingsPage mounted.
//
// Now applyStoredTheme() is invoked from main.tsx BEFORE React renders, so
// the very first paint already matches the user's saved preference.

export type Theme = 'light' | 'dark' | 'system'

const THEME_KEY = 'prism.theme'

const SYSTEM_QUERY = '(prefers-color-scheme: dark)'

function resolve(theme: Theme): 'light' | 'dark' {
  if (theme === 'system') {
    return window.matchMedia(SYSTEM_QUERY).matches ? 'dark' : 'light'
  }
  return theme
}

function applyResolved(resolved: 'light' | 'dark') {
  const root = document.documentElement
  root.classList.remove('dark', 'light')
  root.classList.add(resolved)
  // colour-scheme tells the UA which form-control / scrollbar palette to
  // pick; without it native scrollbars in dark mode look pale grey.
  root.style.colorScheme = resolved
}

export function getStoredTheme(): Theme {
  const raw = (typeof localStorage !== 'undefined' && localStorage.getItem(THEME_KEY)) || ''
  return raw === 'light' || raw === 'dark' || raw === 'system' ? raw : 'dark'
}

export function setStoredTheme(theme: Theme) {
  localStorage.setItem(THEME_KEY, theme)
}

export function applyTheme(theme: Theme) {
  applyResolved(resolve(theme))
}

let mediaUnsub: (() => void) | null = null

// trackSystemTheme keeps the DOM class in sync with the OS appearance while
// the user has Theme=system selected. Returns an unsubscribe function. Only
// the first listener matters: we replace any prior listener so we don't pile
// up handlers across re-renders.
export function trackSystemTheme(active: boolean) {
  if (mediaUnsub) {
    mediaUnsub()
    mediaUnsub = null
  }
  if (!active) return
  const mq = window.matchMedia(SYSTEM_QUERY)
  const handler = () => applyResolved(mq.matches ? 'dark' : 'light')
  mq.addEventListener('change', handler)
  mediaUnsub = () => mq.removeEventListener('change', handler)
}

// applyStoredTheme is the bootstrap call: sync DOM class + colour-scheme
// from the persisted preference before the React tree renders.
export function applyStoredTheme() {
  const t = getStoredTheme()
  applyTheme(t)
  trackSystemTheme(t === 'system')
}
