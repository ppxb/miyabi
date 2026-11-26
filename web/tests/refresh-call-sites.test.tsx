import { renderToStaticMarkup } from 'react-dom/server'
import {
  QueryClient,
  QueryClientProvider,
  useMutation,
  type UseMutationOptions
} from '@tanstack/react-query'
import { expect, test, vi } from 'vitest'

import { useStartLibraryScan } from '@/api/library'
import { useSaveSubtitle } from '@/api/subtitles'
import { TaskConnectionContext, type TaskConnection } from '@/api/task-stream'

vi.mock('@tanstack/react-query', async importOriginal => ({
  ...(await importOriginal<typeof import('@tanstack/react-query')>()),
  useMutation: vi.fn()
}))

type CapturedOptions = UseMutationOptions<unknown, Error, unknown, unknown>
type LooseOptions = { onSuccess?: (...args: unknown[]) => unknown }

const connected: TaskConnection = { status: 'connected', reconnect: () => {} }
const disconnected: TaskConnection = { status: 'disconnected', reconnect: () => {} }

function captureOptions(
  client: QueryClient,
  useOperation: () => unknown,
  connection: TaskConnection | null
) {
  function CaptureOperation() {
    useOperation()
    return null
  }
  renderToStaticMarkup(
    <QueryClientProvider client={client}>
      <TaskConnectionContext value={connection}>
        <CaptureOperation />
      </TaskConnectionContext>
    </QueryClientProvider>
  )
  return vi.mocked(useMutation).mock.calls.at(-1)![0] as CapturedOptions
}

function trackInvalidations(client: QueryClient) {
  const keys: unknown[] = []
  vi.spyOn(client, 'invalidateQueries').mockImplementation(async filters => {
    keys.push(filters?.queryKey)
  })
  return keys
}

test('a connected scan start lets the SSE snapshot refresh tasks', async () => {
  const client = new QueryClient()
  const keys = trackInvalidations(client)
  const options = captureOptions(client, useStartLibraryScan, connected) as unknown as LooseOptions

  await options.onSuccess?.()
  expect(keys).toEqual([])
})

test('a disconnected scan start falls back to an HTTP task refresh', async () => {
  const client = new QueryClient()
  const keys = trackInvalidations(client)
  const options = captureOptions(
    client,
    useStartLibraryScan,
    disconnected
  ) as unknown as LooseOptions

  await options.onSuccess?.()
  expect(keys).toEqual([['tasks']])
})

test('subtitle save refreshes only the queries that render the subtitle badge', async () => {
  const client = new QueryClient()
  const keys = trackInvalidations(client)
  const options = captureOptions(
    client,
    () => useSaveSubtitle(7),
    connected
  ) as unknown as LooseOptions

  await options.onSuccess?.()
  expect(new Set(keys)).toEqual(
    new Set([
      ['subtitles', 7, 'tracks'],
      ['library', 'detail', 7],
      ['history'],
      ['library', 'movies'],
      ['library', 'browse'],
      ['library', 'top-rated']
    ])
  )
})
