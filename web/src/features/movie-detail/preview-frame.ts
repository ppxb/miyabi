import { FIT_VIEW, type ZoomView } from './preview-zoom'

// Accumulate every gesture delta, but publish only the latest view each frame.
export function createPreviewFrame(publish: (view: ZoomView) => void) {
  let current = FIT_VIEW
  let frame: number | undefined

  return {
    current: () => current,
    update(update: (view: ZoomView) => ZoomView) {
      const next = update(current)
      if (next.scale === current.scale && next.x === current.x && next.y === current.y) return
      current = next
      if (frame !== undefined) return
      frame = requestAnimationFrame(() => {
        frame = undefined
        publish(current)
      })
    },
    cancel() {
      if (frame !== undefined) cancelAnimationFrame(frame)
      frame = undefined
    }
  }
}
