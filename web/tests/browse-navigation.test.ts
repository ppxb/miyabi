import assert from 'node:assert/strict'
import { test } from 'vitest'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter
} from '@tanstack/react-router'

import {
  validateDiscoverSearch,
  validateMainSearch,
  categoryFromSearch
} from '@/features/discover/search'
import { validateMetadataSearch } from '@/features/discover/metadata-search'
import { validateSubscriptionsSearch } from '@/features/subscriptions/search'

function routerAt(href: string) {
  const root = createRootRoute()
  const discover = createRoute({
    getParentRoute: () => root,
    path: '/discover',
    validateSearch: validateDiscoverSearch
  })
  const detail = createRoute({
    getParentRoute: () => root,
    path: '/discover/$movieId',
    validateSearch: validateMainSearch
  })
  const metadata = createRoute({
    getParentRoute: () => root,
    path: '/discover/search',
    validateSearch: validateMetadataSearch
  })
  const subscriptions = createRoute({
    getParentRoute: () => root,
    path: '/subscriptions',
    validateSearch: validateSubscriptionsSearch
  })
  return createRouter({
    isServer: false,
    origin: 'http://localhost',
    routeTree: root.addChildren([discover, detail, metadata, subscriptions]),
    history: createMemoryHistory({ initialEntries: [href] })
  })
}

test('discover filters and all tab pages survive detail navigation, back and reload', async () => {
  const href =
    '/discover?view=category&zone=uncensored&categoryID=year&tagID=2026&main=m&categoryPage=3&releasedPage=4&upcomingPage=2'
  const router = routerAt(href)
  await router.load()
  assert.equal(router.state.matches.at(-1)!.status, 'success')
  const original = router.state.matches.at(-1)!.search
  await router.navigate({
    to: '/discover/$movieId',
    params: { movieId: 'one' },
    search: previous => ({ main: previous.main })
  })
  await router.load()
  assert.equal(router.state.location.search.main, 'm')
  assert.equal(router.state.location.search.categoryPage, undefined)
  router.history.back()
  await router.load()
  assert.deepEqual(router.state.matches.at(-1)!.search, original)
  const fresh = routerAt(router.state.location.href)
  await fresh.load()
  assert.deepEqual(fresh.state.matches.at(-1)!.search, original)
  assert.deepEqual(categoryFromSearch(validateDiscoverSearch(original)), {
    zone: 'uncensored',
    categoryID: 'year',
    tagID: '2026',
    main: 'm'
  })
})

test('subscription actor selection and both pages survive back and reload', async () => {
  const router = routerAt('/subscriptions?view=actors&actorID=7&page=3&actorPage=2')
  await router.load()
  assert.equal(router.state.matches.at(-1)!.status, 'success')
  const original = router.state.matches.at(-1)!.search
  await router.navigate({ to: '/discover/$movieId', params: { movieId: 'one' } })
  await router.load()
  router.history.back()
  await router.load()
  assert.deepEqual(router.state.matches.at(-1)!.search, original)
  const fresh = routerAt(router.state.location.href)
  await fresh.load()
  assert.deepEqual(validateSubscriptionsSearch(fresh.state.matches.at(-1)!.search), {
    view: 'actors',
    actorID: 7,
    page: 3,
    actorPage: 2
  })
})

test('metadata filters travel through detail links without shared mutable state', async () => {
  const router = routerAt('/discover/search?kind=actor&id=actor-1&name=Actor&main=c&page=2')
  await router.load()
  await router.navigate({
    to: '/discover/$movieId',
    params: { movieId: 'one' },
    search: previous => ({ main: previous.main })
  })
  await router.load()
  await router.navigate({
    to: '/discover/search',
    search: previous => ({
      kind: 'maker',
      id: 'maker-1',
      name: 'Maker',
      page: 1,
      main: previous.main ?? ''
    })
  })
  await router.load()
  const search = validateMetadataSearch(router.state.matches.at(-1)!.search)
  assert.equal(search.main, 'c')
  assert.equal(search.page, 1)
  const independent = routerAt('/discover')
  await independent.load()
  assert.equal(validateDiscoverSearch(independent.state.matches.at(-1)!.search).main, undefined)
})

test('browse validators reject malformed URL state and omit default pages', () => {
  for (const search of [
    { view: 'other' },
    { zone: 'other' },
    { main: 'other' },
    { categoryID: [] },
    { tagID: {} },
    { categoryPage: -1 },
    { releasedPage: 1.5 },
    { upcomingPage: Infinity }
  ]) {
    assert.throws(() => validateDiscoverSearch(search))
  }
  for (const search of [
    { view: 'other' },
    { view: 'actors', actorID: 0 },
    { view: 'actors', actorID: 1.5 },
    { page: -1 },
    { actorPage: Infinity }
  ]) {
    assert.throws(() => validateSubscriptionsSearch(search))
  }
  assert.equal(validateDiscoverSearch({ categoryPage: 1 }).categoryPage, undefined)
  assert.equal(validateSubscriptionsSearch({ page: 1 }).page, undefined)
  assert.equal(validateSubscriptionsSearch({ view: 'movies', actorID: 7 }).actorID, undefined)
})
