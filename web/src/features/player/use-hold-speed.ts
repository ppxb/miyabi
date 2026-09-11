import type { MediaPlayerInstance } from '@vidstack/react'
import { useEffect, useState } from 'react'

const holdDelay = 500
const ignoredTargets =
  'input, textarea, select, [contenteditable]:not([contenteditable="false"]), [data-slot="select-trigger"], [data-slot="select-content"], .vds-menu, .vds-menu-items, .vds-slider'

export function useHoldSpeed(player: MediaPlayerInstance | null) {
  const [active, setActive] = useState(false)

  useEffect(() => {
    if (player) return bindHoldSpeed(player, setActive)
  }, [player])

  return active
}

export function bindHoldSpeed(
  player: MediaPlayerInstance,
  onActiveChange: (active: boolean) => void
) {
  const element = player.el
  const document = element?.ownerDocument
  const window = document?.defaultView
  if (!element || !document || !window) return () => {}

  const listeners = new AbortController()
  const options = { signal: listeners.signal }
  let held = false
  let pending = false
  let forwarding = false
  let previousRate: number | undefined
  let timer: ReturnType<typeof setTimeout> | undefined

  function cancel() {
    clearTimeout(timer)
    timer = undefined
    pending = false
    if (previousRate !== undefined) {
      const rate = previousRate
      previousRate = undefined
      // The setter also restores a rate whose native ratechange event is still pending.
      player.playbackRate = rate
      onActiveChange(false)
    }
    // Keep ownership until keyup so a cancelled hold cannot become a normal seek.
  }

  function consume(event: KeyboardEvent) {
    event.preventDefault()
    event.stopImmediatePropagation()
  }

  function keyDown(event: KeyboardEvent) {
    if (forwarding || event.key !== 'ArrowRight') return
    if (event.repeat) {
      if (held) consume(event)
      return
    }

    cancel()
    held = false
    const target = event.target as Element | null
    if (
      !player.state.canPlay ||
      event.altKey ||
      event.ctrlKey ||
      event.metaKey ||
      event.shiftKey ||
      event.isComposing ||
      target?.closest(ignoredTargets)
    )
      return

    consume(event)
    held = true
    pending = true
    timer = setTimeout(() => {
      timer = undefined
      pending = false
      if (!player.state.canPlay || !player.state.canSetPlaybackRate) return
      const rate = player.playbackRate
      if (!Number.isFinite(rate) || rate <= 0) return
      previousRate = rate
      player.playbackRate = 3
      onActiveChange(true)
    }, holdDelay)
  }

  const keyUp = (event: KeyboardEvent) => {
    if (forwarding || event.key !== 'ArrowRight' || !held) return
    consume(event)
    const seek = pending
    held = false
    cancel()
    if (
      seek &&
      player.state.canSeek &&
      Number.isFinite(player.currentTime) &&
      Number.isFinite(player.duration) &&
      player.duration > 0
    ) {
      // Delegate confirmed taps to Vidstack so seeking and its keyboard feedback stay together.
      forwarding = true
      try {
        for (const type of ['keydown', 'keyup']) {
          element.dispatchEvent(
            new window.KeyboardEvent(type, {
              key: 'ArrowRight',
              code: 'ArrowRight',
              bubbles: true,
              cancelable: true
            })
          )
        }
      } finally {
        forwarding = false
      }
    }
  }

  element.addEventListener('keydown', keyDown, { ...options, capture: true })
  window.addEventListener('keyup', keyUp, { ...options, capture: true })
  window.addEventListener('blur', cancel, options)
  window.addEventListener('pagehide', cancel, options)
  document.addEventListener(
    'visibilitychange',
    () => {
      if (document.hidden) cancel()
    },
    options
  )
  for (const type of [
    'focusout',
    'pause',
    'ended',
    'error',
    'source-change',
    'provider-change',
    'emptied'
  ]) {
    element.addEventListener(type, cancel, options)
  }

  return () => {
    cancel()
    held = false
    listeners.abort()
  }
}
