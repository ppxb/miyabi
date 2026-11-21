import { useEffect, useSyncExternalStore } from 'react'
import { create } from 'zustand'
import { persist } from 'zustand/middleware'

export type ThemeMode = 'light' | 'dark' | 'system'

type ThemeState = {
  theme: ThemeMode
  setTheme: (theme: ThemeMode) => void
}

export const useThemeStore = create<ThemeState>()(
  persist(
    set => ({
      theme: 'system',
      setTheme: theme => set({ theme })
    }),
    {
      name: 'miyabi-theme'
    }
  )
)

const THEME_QUERY = '(prefers-color-scheme: dark)'

function subscribeSystemTheme(onStoreChange: () => void) {
  const list = window.matchMedia(THEME_QUERY)
  list.addEventListener('change', onStoreChange)
  return () => list.removeEventListener('change', onStoreChange)
}

function systemPrefersDark() {
  return window.matchMedia(THEME_QUERY).matches
}

export function useTheme() {
  const theme = useThemeStore(state => state.theme)
  const setTheme = useThemeStore(state => state.setTheme)
  const systemDark = useSyncExternalStore(subscribeSystemTheme, systemPrefersDark)
  const resolvedTheme = theme === 'system' ? (systemDark ? 'dark' : 'light') : theme

  return { theme, setTheme, resolvedTheme }
}

export function ThemeSync() {
  const { theme, resolvedTheme } = useTheme()

  useEffect(() => {
    const root = document.documentElement
    root.setAttribute('data-theme-mode', theme)
    // Disable transitions for a frame so switching themes does not animate every colour.
    root.classList.add('disable-transitions')
    root.classList.toggle('dark', resolvedTheme === 'dark')
    root.style.colorScheme = resolvedTheme
    requestAnimationFrame(() => root.classList.remove('disable-transitions'))
  }, [theme, resolvedTheme])

  return null
}
