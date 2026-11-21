import { clamp } from '@/lib/math'

export type Size = { width: number; height: number }

/** A point measured from the center of the stage. */
export type Point = { x: number; y: number }

/** Image box relative to the center of the stage: `translate(x, y) scale(scale)`. */
export type ZoomView = { scale: number; x: number; y: number }

export const MIN_SCALE = 0.5
export const MAX_SCALE = 8
export const DOUBLE_TAP_SCALE = 2.5

/** The scale of the opening view: the whole image, never larger than its own pixels. */
export const FIT_SCALE = 1

/**
 * Shared instance for the fit state, so repeated resets keep the same reference and React can
 * bail out of a re-render.
 */
export const FIT_VIEW: ZoomView = { scale: FIT_SCALE, x: 0, y: 0 }

/** Preview frames are video stills, so guess 16:9 until the original reports its own size. */
export const PREVIEW_FALLBACK_SIZE: Size = { width: 16, height: 9 }

/** Larger step means slower zoom. Trackpad pinch arrives as ctrl+wheel with much smaller deltas. */
export const WHEEL_ZOOM_STEP = 500
export const PINCH_ZOOM_STEP = 100

const LINE_DELTA = 16
const PAGE_DELTA = 100

export function clampScale(scale: number) {
  return clamp(scale, MIN_SCALE, MAX_SCALE)
}

/** Size the image occupies once contained (object-contain) inside the stage. */
export function containSize(natural: Size, stage: Size, allowUpscale = false): Size {
  if (natural.width <= 0 || natural.height <= 0) return { width: stage.width, height: stage.height }
  if (stage.width <= 0 || stage.height <= 0) return { width: 0, height: 0 }
  // Preview originals are small; stretching them to fill a monitor only magnifies blur.
  const cap = allowUpscale ? Number.POSITIVE_INFINITY : 1
  const ratio = Math.min(stage.width / natural.width, stage.height / natural.height, cap)
  return { width: natural.width * ratio, height: natural.height * ratio }
}

export function previewBase(natural: Size | null, stage: Size): Size {
  // Until the original reports its size, the placeholder fills the stage at the preview aspect.
  if (!natural) return containSize(PREVIEW_FALLBACK_SIZE, stage, true)
  return containSize(natural, stage)
}

/** How far the zoomed image may travel before it would expose a gap along that axis. */
function axisLimit(base: number, stage: number, scale: number) {
  return Math.max(0, (base * scale - stage) / 2)
}

export function clampView(view: ZoomView, base: Size, stage: Size): ZoomView {
  const scale = clampScale(view.scale)
  const limitX = axisLimit(base.width, stage.width, scale)
  const limitY = axisLimit(base.height, stage.height, scale)
  const x = clamp(view.x, -limitX, limitX)
  const y = clamp(view.y, -limitY, limitY)
  // Keep the same object when nothing moved, so React can skip the re-render.
  if (scale === view.scale && x === view.x && y === view.y) return view
  return { scale, x, y }
}

/** Whether the current zoom leaves anything to pan: a fitted image is already centered. */
export function canPan(view: ZoomView, base: Size, stage: Size) {
  return (
    axisLimit(base.width, stage.width, view.scale) > 0 ||
    axisLimit(base.height, stage.height, view.scale) > 0
  )
}

/** Zoom by `factor`, keeping the image point under `anchor` pinned to that spot on screen. */
export function zoomAt(
  view: ZoomView,
  anchor: Point,
  factor: number,
  base: Size,
  stage: Size
): ZoomView {
  const current = clampScale(view.scale)
  const scale = clampScale(current * factor)
  const ratio = scale / current
  return clampView(
    {
      scale,
      x: anchor.x - ratio * (anchor.x - view.x),
      y: anchor.y - ratio * (anchor.y - view.y)
    },
    base,
    stage
  )
}

export function panBy(view: ZoomView, dx: number, dy: number, base: Size, stage: Size): ZoomView {
  return clampView({ scale: view.scale, x: view.x + dx, y: view.y + dy }, base, stage)
}

/** Wheel deltas arrive in pixels, lines or pages depending on the device and the browser. */
export function normalizeWheelDelta(deltaY: number, deltaMode: number) {
  if (deltaMode === 1) return deltaY * LINE_DELTA
  if (deltaMode === 2) return deltaY * PAGE_DELTA
  return deltaY
}

export function wheelZoomFactor(deltaY: number, deltaMode = 0, step = WHEEL_ZOOM_STEP) {
  return Math.exp(-normalizeWheelDelta(deltaY, deltaMode) / step)
}

/** Arrows and the thumbnail strip wrap around, so the index always stays inside the list. */
export function stepIndex(index: number, length: number, step: number) {
  if (length <= 0) return 0
  return (((index + step) % length) + length) % length
}
