import assert from 'node:assert/strict'
import type { ComponentProps, MouseEvent } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { test, vi } from 'vitest'

import { MovieDetailDialogContext, MovieDetailLink } from '@/features/movie-detail/detail-link'

const { link } = vi.hoisted(() => ({ link: vi.fn() }))
vi.mock('@tanstack/react-router', () => ({
  Link: (props: unknown) => {
    link(props)
    return null
  }
}))

function renderLink(props: Partial<ComponentProps<typeof MovieDetailLink>> = {}) {
  const openMovie = vi.fn()
  renderToStaticMarkup(
    <MovieDetailDialogContext value={openMovie}>
      <MovieDetailLink movieId="movie-1" {...props}>
        Movie
      </MovieDetailLink>
    </MovieDetailDialogContext>
  )
  return {
    openMovie,
    props: link.mock.lastCall![0] as {
      to: string
      params: { movieId: string }
      search: (previous: { main?: string }) => { main?: string }
      onClick: (event: MouseEvent<HTMLAnchorElement>) => void
    }
  }
}

function click(overrides: Partial<MouseEvent<HTMLAnchorElement>> = {}) {
  const event = {
    button: 0,
    metaKey: false,
    ctrlKey: false,
    shiftKey: false,
    altKey: false,
    defaultPrevented: false,
    currentTarget: { target: '' },
    preventDefault() {
      this.defaultPrevented = true
    },
    ...overrides
  }
  return event as MouseEvent<HTMLAnchorElement>
}

test('a normal card click opens the dialog without navigating and retains a direct detail URL', () => {
  const { openMovie, props } = renderLink()
  const event = click()
  props.onClick(event)
  assert.equal(event.defaultPrevented, true)
  assert.equal(openMovie.mock.calls.length, 1)
  assert.deepEqual(openMovie.mock.calls[0], ['movie-1', event.currentTarget])
  assert.equal(props.to, '/discover/$movieId')
  assert.deepEqual(props.params, { movieId: 'movie-1' })
  assert.deepEqual(props.search({ main: 'c' }), { main: 'c' })
})

test('card actions that prevent navigation also prevent opening the dialog', () => {
  const { openMovie, props } = renderLink()
  const event = click({ defaultPrevented: true })
  props.onClick(event)
  assert.equal(openMovie.mock.calls.length, 0)

  const intercepted = renderLink({ onClick: event => event.preventDefault() })
  intercepted.props.onClick(click())
  assert.equal(intercepted.openMovie.mock.calls.length, 0)
})

test('modified clicks and new-tab targets keep normal link behavior', () => {
  const { openMovie, props } = renderLink()
  for (const overrides of [
    { ctrlKey: true },
    { metaKey: true },
    { shiftKey: true },
    { altKey: true },
    { button: 1 },
    { currentTarget: { target: '_blank' } as HTMLAnchorElement }
  ]) {
    const event = click(overrides)
    props.onClick(event)
    assert.equal(event.defaultPrevented, false)
  }
  assert.equal(openMovie.mock.calls.length, 0)
})

test('recommendation prioritization runs before opening the selected movie', () => {
  const prioritize = vi.fn()
  const { openMovie, props } = renderLink({ movieId: 'recommended', onClick: prioritize })
  const event = click()
  props.onClick(event)
  assert.equal(prioritize.mock.calls.length, 1)
  assert.deepEqual(openMovie.mock.calls[0], ['recommended', event.currentTarget])
  assert.ok(prioritize.mock.invocationCallOrder[0]! < openMovie.mock.invocationCallOrder[0]!)
})
