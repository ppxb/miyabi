import { useEffect, useEffectEvent, useState } from 'react'

/**
 * Keeps a search input in sync with a URL query value. Typing is submitted
 * trimmed after a 300ms debounce; external URL changes (back/forward) are
 * reflected into the input without erasing what the user is still typing.
 */
export function useDebouncedSearch(
  value: string | undefined,
  submit: (q: string | undefined) => void
) {
  const external = value ?? ''
  const [query, setQuery] = useState(external)
  const [synced, setSynced] = useState(external)

  // Skip the write when the URL merely echoes our own trimmed submission
  // ("abc " -> "abc"), so trailing spaces being typed survive.
  if (external !== synced) {
    setSynced(external)
    if (external !== query.trim()) {
      setQuery(external)
    }
  }

  // The latest submit is read when the timer fires, so callers may pass an
  // inline callback without resetting the debounce on every render.
  const onSubmit = useEffectEvent(submit)
  useEffect(() => {
    if (query.trim() === external) return
    const timer = setTimeout(() => onSubmit(query.trim() || undefined), 300)
    return () => clearTimeout(timer)
  }, [query, external])

  return [query, setQuery] as const
}
