import { expect, test, vi } from 'vitest'

import { createPreviewFrame } from '@/features/movie-detail/preview-frame'
import { FIT_VIEW, panBy, zoomAt } from '@/features/movie-detail/preview-zoom'

function frameClock() {
  const pending = new Map<number, FrameRequestCallback>()
  let nextID = 0
  const request = vi.fn((callback: FrameRequestCallback) => {
    const id = nextID++
    pending.set(id, callback)
    return id
  })
  vi.stubGlobal('requestAnimationFrame', request)
  vi.stubGlobal('cancelAnimationFrame', (id: number) => pending.delete(id))
  return {
    request,
    flush() {
      const callbacks = [...pending.values()]
      pending.clear()
      callbacks.forEach(callback => callback(0))
    }
  }
}

test('preview gestures keep every delta while publishing only once per frame', () => {
  const clock = frameClock()
  const publish = vi.fn()
  const frame = createPreviewFrame(publish)
  const size = { width: 1000, height: 600 }
  let expected = FIT_VIEW
  for (let i = 0; i < 20; i++) {
    const anchor = { x: i * 2, y: i }
    expected = zoomAt(expected, anchor, 1.02, size, size)
    frame.update(current => zoomAt(current, anchor, 1.02, size, size))
  }
  expected = panBy(expected, 10, -5, size, size)
  frame.update(current => panBy(current, 10, -5, size, size))
  expect(frame.current()).toEqual(expected)
  expect(clock.request).toHaveBeenCalledOnce()
  expect(publish).not.toHaveBeenCalled()
  clock.flush()
  expect(publish).toHaveBeenCalledExactlyOnceWith(expected)

  frame.update(() => FIT_VIEW)
  clock.flush()
  expect(publish).toHaveBeenLastCalledWith(FIT_VIEW)
  expect(publish).toHaveBeenCalledTimes(2)
})

test('stationary and clamped gestures do not schedule unnecessary frames', () => {
  const clock = frameClock()
  const frame = createPreviewFrame(vi.fn())
  const size = { width: 1000, height: 600 }
  frame.update(current => panBy(current, 100, 100, size, size))
  frame.update(current => zoomAt(current, { x: 0, y: 0 }, 1, size, size))
  expect(clock.request).not.toHaveBeenCalled()
})

test('changing images cancels a pending frame, including frame ID zero', () => {
  const clock = frameClock()
  const publish = vi.fn()
  const frame = createPreviewFrame(publish)
  frame.update(() => ({ scale: 2, x: 0, y: 0 }))
  frame.cancel()
  clock.flush()
  expect(publish).not.toHaveBeenCalled()
  // Effect cleanup/replay can reuse the controller in development StrictMode.
  frame.update(() => FIT_VIEW)
  clock.flush()
  expect(publish).toHaveBeenCalledExactlyOnceWith(FIT_VIEW)
})
