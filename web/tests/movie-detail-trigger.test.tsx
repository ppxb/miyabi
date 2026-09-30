import assert from 'node:assert/strict'
import type { ComponentProps, KeyboardEvent, MouseEvent } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { test, vi } from 'vitest'

import { MovieDetailTrigger } from '@/features/movie-detail/detail-trigger'
import { MovieDetailDialogContext } from '@/features/movie-detail/dialog-context'

function renderTrigger(overrides: Partial<ComponentProps<typeof MovieDetailTrigger>> = {}) {
  const openMovie = vi.fn()
  let props!: ComponentProps<'div'>
  function CaptureTrigger() {
    const element = MovieDetailTrigger({ movieId: 'movie-1', children: 'Movie', ...overrides })
    props = element.props
    return element
  }
  const html = renderToStaticMarkup(
    <MovieDetailDialogContext value={openMovie}>
      <CaptureTrigger />
    </MovieDetailDialogContext>
  )
  return { openMovie, props, html }
}

function click(overrides: Partial<MouseEvent<HTMLDivElement>> = {}) {
  const event = {
    defaultPrevented: false,
    currentTarget: {},
    preventDefault() {
      this.defaultPrevented = true
    },
    ...overrides
  }
  return event as MouseEvent<HTMLDivElement>
}

test('movie cards open the dialog without rendering a navigation link', () => {
  const { openMovie, props, html } = renderTrigger()
  assert.match(html, /^<div /)
  assert.match(html, /role="button"/)
  assert.match(html, /tabindex="0"/)
  assert.doesNotMatch(html, /href=|<a\b/)
  const event = click()
  props.onClick?.(event)
  assert.equal(event.defaultPrevented, true)
  assert.deepEqual(openMovie.mock.calls, [['movie-1', event.currentTarget]])
})

test('card actions and selection can consume a click without opening details', () => {
  const { openMovie, props } = renderTrigger()
  props.onClick?.(click({ defaultPrevented: true }))
  assert.equal(openMovie.mock.calls.length, 0)
  const select = vi.fn((event: MouseEvent<HTMLDivElement>) => event.preventDefault())
  const selection = renderTrigger({ onClick: select, role: 'checkbox', 'aria-haspopup': false })
  selection.props.onClick?.(click())
  assert.equal(select.mock.calls.length, 1)
  assert.equal(selection.openMovie.mock.calls.length, 0)
  assert.equal(selection.props['aria-haspopup'], false)
})

test('Enter and Space activate the card once and ignore keyboard events from child controls', () => {
  const { openMovie, props } = renderTrigger()
  const currentTarget = { click: () => props.onClick?.(click()) } as HTMLDivElement
  for (const key of ['Enter', ' ']) {
    const preventDefault = vi.fn()
    props.onKeyDown?.({
      key,
      currentTarget,
      target: currentTarget,
      preventDefault
    } as unknown as KeyboardEvent<HTMLDivElement>)
    assert.equal(preventDefault.mock.calls.length, 1)
  }
  assert.equal(openMovie.mock.calls.length, 2)
  for (const overrides of [{ repeat: true }, { target: {} }]) {
    props.onKeyDown?.({
      key: ' ',
      currentTarget,
      target: currentTarget,
      preventDefault: vi.fn(),
      ...overrides
    } as unknown as KeyboardEvent<HTMLDivElement>)
  }
  assert.equal(openMovie.mock.calls.length, 2)
})

test('disabled cards cannot open details or invoke selection actions', () => {
  const onClick = vi.fn()
  const { openMovie, props } = renderTrigger({ disabled: true, onClick })
  props.onClick?.(click())
  const activate = vi.fn()
  const target = { click: activate }
  props.onKeyDown?.({
    key: 'Enter',
    currentTarget: target,
    target,
    preventDefault: vi.fn()
  } as unknown as KeyboardEvent<HTMLDivElement>)
  assert.equal(onClick.mock.calls.length, 0)
  assert.equal(openMovie.mock.calls.length, 0)
  assert.equal(activate.mock.calls.length, 0)
  assert.equal(props.tabIndex, -1)
  assert.equal(props['aria-disabled'], true)
})

test('recommendations prioritize their request before opening in the dialog', () => {
  const prioritize = vi.fn()
  const { openMovie, props } = renderTrigger({ movieId: 'recommended', onClick: prioritize })
  const event = click()
  props.onClick?.(event)
  assert.deepEqual(openMovie.mock.calls, [['recommended', event.currentTarget]])
  assert.ok(prioritize.mock.invocationCallOrder[0]! < openMovie.mock.invocationCallOrder[0]!)
})
