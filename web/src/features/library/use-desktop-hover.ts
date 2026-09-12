import { useSyncExternalStore } from 'react'

const desktopHoverQuery = '(min-width: 768px) and (hover: hover) and (pointer: fine)'

function subscribe(notify: () => void) {
  const media = window.matchMedia(desktopHoverQuery)
  media.addEventListener('change', notify)
  return () => media.removeEventListener('change', notify)
}

export function useDesktopHover() {
  return useSyncExternalStore(
    subscribe,
    () => window.matchMedia(desktopHoverQuery).matches,
    () => false
  )
}
