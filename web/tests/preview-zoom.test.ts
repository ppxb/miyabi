import assert from 'node:assert/strict'
import { test } from 'vitest'

import {
  type Point,
  type ZoomView,
  FIT_VIEW,
  MAX_SCALE,
  MIN_SCALE,
  PINCH_ZOOM_STEP,
  WHEEL_ZOOM_STEP,
  canPan,
  clampScale,
  clampView,
  containSize,
  normalizeWheelDelta,
  panBy,
  previewBase,
  stepIndex,
  wheelZoomFactor,
  zoomAt
} from '@/features/movie-detail/preview-zoom'

const STAGE = { width: 1200, height: 800 }
const UHD = { width: 1920, height: 1080 }
const WIDE = { width: 1600, height: 900 }

/** Point of the image that sits under `anchor`, in image coordinates. */
function anchoredPoint(view: ZoomView, anchor: Point) {
  return { x: (anchor.x - view.x) / view.scale, y: (anchor.y - view.y) / view.scale }
}

test('containSize fits the image inside the stage and keeps its aspect ratio', () => {
  assert.deepEqual(containSize(UHD, STAGE), { width: 1200, height: 675 })
  assert.deepEqual(containSize({ width: 1000, height: 2000 }, STAGE), { width: 400, height: 800 })
  const tall = containSize({ width: 1000, height: 2000 }, STAGE)
  assert.equal(tall.width / tall.height, 0.5)
  assert.ok(tall.width <= STAGE.width && tall.height <= STAGE.height)
})

test('containSize never magnifies a small original past its own pixels', () => {
  assert.deepEqual(containSize({ width: 400, height: 225 }, STAGE), { width: 400, height: 225 })
  assert.deepEqual(containSize({ width: 400, height: 225 }, STAGE, true), {
    width: 1200,
    height: 675
  })
})

test('containSize falls back to the stage for missing sizes', () => {
  assert.deepEqual(containSize({ width: 0, height: 0 }, STAGE), STAGE)
  assert.deepEqual(containSize(UHD, { width: 0, height: 0 }), { width: 0, height: 0 })
})

test('previewBase fills the stage until the original reports its size', () => {
  assert.deepEqual(previewBase(null, STAGE), { width: 1200, height: 675 })
  assert.deepEqual(previewBase(UHD, STAGE), { width: 1200, height: 675 })
  assert.deepEqual(previewBase({ width: 400, height: 225 }, STAGE), { width: 400, height: 225 })
  assert.deepEqual(previewBase(null, { width: 0, height: 0 }), { width: 0, height: 0 })
})

test('zoomAt keeps the point under the cursor pinned in place', () => {
  const base = containSize(WIDE, STAGE)
  const anchor = { x: 300, y: -150 }
  const zoomed = zoomAt(FIT_VIEW, anchor, 2, base, STAGE)

  assert.deepEqual(zoomed, { scale: 2, x: -300, y: 150 })
  assert.deepEqual(anchoredPoint(zoomed, anchor), anchoredPoint(FIT_VIEW, anchor))
})

test('zooming out stops at the minimum scale and re-centers the image', () => {
  const base = containSize(WIDE, STAGE)
  const zoomed = zoomAt(FIT_VIEW, { x: 0, y: 0 }, 4, base, STAGE)
  assert.equal(zoomed.scale, 4)

  const small = zoomAt(zoomed, { x: 120, y: 90 }, 0.01, base, STAGE)
  assert.deepEqual(small, { scale: MIN_SCALE, x: 0, y: 0 })
  assert.equal(clampScale(0.4), MIN_SCALE)
  assert.equal(clampScale(1000), MAX_SCALE)
})

test('clampView caps zoom, keeps the anchor and drops the pan when it cannot zoom', () => {
  const base = containSize(WIDE, STAGE)
  assert.deepEqual(clampView({ scale: 100, x: 5000, y: 5000 }, base, STAGE), {
    scale: MAX_SCALE,
    x: 4200,
    y: 2300
  })
  assert.deepEqual(clampView({ scale: 1, x: 120, y: -80 }, base, STAGE), { scale: 1, x: 0, y: 0 })
  // Nothing moved, so the very same object comes back and React can skip a render.
  const fitted = { scale: 1.5, x: 0, y: 0 }
  assert.equal(clampView(fitted, base, STAGE), fitted)
})

test('canPan tracks whether the zoomed image is larger than the stage', () => {
  const base = containSize(WIDE, STAGE)
  assert.equal(canPan(FIT_VIEW, base, STAGE), false)
  assert.equal(canPan({ scale: 0.5, x: 0, y: 0 }, base, STAGE), false)
  assert.equal(canPan({ scale: 2, x: 0, y: 0 }, base, STAGE), true)
})

test('panning never exposes a gap and is a no-op while fitted', () => {
  const base = containSize(WIDE, STAGE)
  assert.deepEqual(panBy(FIT_VIEW, 100, 100, base, STAGE), { scale: 1, x: 0, y: 0 })
  assert.deepEqual(panBy({ scale: 2, x: 0, y: 0 }, 10_000, 10_000, base, STAGE), {
    scale: 2,
    x: 600,
    y: 275
  })
  // Letterboxed axis: the image is shorter than the stage until it is zoomed far enough.
  assert.deepEqual(panBy({ scale: 1.1, x: 0, y: 0 }, 100, 100, base, STAGE), {
    scale: 1.1,
    x: 60,
    y: 0
  })
})

test('every clamped view either covers the stage or stays centered on that axis', () => {
  const base = containSize(WIDE, STAGE)
  for (const scale of [MIN_SCALE, 0.8, 1, 1.2, 2, 3.5, MAX_SCALE]) {
    for (const offset of [0, 50, 600, 5000, -5000]) {
      const view = clampView({ scale, x: offset, y: offset }, base, STAGE)
      const width = base.width * view.scale
      const height = base.height * view.scale
      const covers = (extent: number, stage: number, value: number) =>
        extent >= stage
          ? value - extent / 2 <= -stage / 2 + 1e-9 && value + extent / 2 >= stage / 2 - 1e-9
          : value === 0
      assert.ok(covers(width, STAGE.width, view.x), `x gap at scale ${scale}, offset ${offset}`)
      assert.ok(covers(height, STAGE.height, view.y), `y gap at scale ${scale}, offset ${offset}`)
    }
  }
})

test('wheel deltas are normalized across pixels, lines and pages', () => {
  assert.equal(normalizeWheelDelta(12, 0), 12)
  assert.equal(normalizeWheelDelta(3, 1), 48)
  assert.equal(normalizeWheelDelta(1, 2), 100)
})

test('scrolling up zooms in, scrolling down zooms out, and pinch is stronger', () => {
  assert.equal(wheelZoomFactor(0), 1)
  assert.ok(wheelZoomFactor(-100) > 1)
  assert.ok(wheelZoomFactor(100) < 1)
  const pinch = wheelZoomFactor(4, 0, PINCH_ZOOM_STEP)
  const wheel = wheelZoomFactor(4, 0, WHEEL_ZOOM_STEP)
  assert.ok(pinch < wheel && wheel < 1)
})

test('stepIndex wraps in both directions and stays safe for an empty list', () => {
  assert.equal(stepIndex(0, 5, -1), 4)
  assert.equal(stepIndex(4, 5, 1), 0)
  assert.equal(stepIndex(2, 5, 1), 3)
  assert.equal(stepIndex(3, 5, 0), 3)
  assert.equal(stepIndex(0, 0, 1), 0)
})
