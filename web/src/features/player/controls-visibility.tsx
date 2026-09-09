import { useEffect } from 'react'
import {
  useMediaPlayer,
  useMediaState,
  type MediaControls,
  type MediaPauseControlsRequestEvent
} from '@vidstack/react'

export function PlayerControlsVisibility() {
  const player = useMediaPlayer()
  const pointer = useMediaState('pointer')

  useEffect(() => {
    if (!player?.el || pointer !== 'fine') return
    return trackPointerControls(player.el, player.controls)
  }, [player, pointer])

  return null
}

function trackPointerControls(element: HTMLElement, controls: MediaControls) {
  const canIdle = controls.canIdle
  const listeners = new AbortController()
  const options = { signal: listeners.signal }

  // Keep Vidstack's visibility state and timers, but wake desktop controls only on pointer movement.
  controls.canIdle = false
  controls.hide(0)

  element.addEventListener(
    'pointermove',
    event => {
      if (event.pointerType === 'touch') return
      controls.show(0, event)
      controls.hide(undefined, event)
    },
    options
  )
  element.addEventListener('pointerleave', event => controls.hide(0, event), options)
  element.addEventListener('pointerup', event => controls.hide(undefined, event), options)

  // Keyboard seeking is forwarded to the time slider, which also requests visible controls.
  const keepKeyboardVisibility = (event: Event) => {
    const { triggers } = event as MediaPauseControlsRequestEvent
    if (triggers.hasType('keydown') || triggers.hasType('keyup')) event.preventDefault()
  }
  for (const name of ['media-pause-controls-request', 'media-resume-controls-request']) {
    element.addEventListener(name, keepKeyboardVisibility, { ...options, capture: true })
  }

  return () => {
    listeners.abort()
    controls.canIdle = canIdle
  }
}
