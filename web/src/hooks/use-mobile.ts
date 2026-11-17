import * as React from 'react'

const MOBILE_BREAKPOINT = 768

export function useIsMobile() {
  const query = `(max-width: ${MOBILE_BREAKPOINT - 1}px)`

  return React.useSyncExternalStore(
    onStoreChange => {
      const list = window.matchMedia(query)
      list.addEventListener('change', onStoreChange)
      return () => list.removeEventListener('change', onStoreChange)
    },
    () => window.matchMedia(query).matches,
    () => false
  )
}
