import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Children, isValidElement, type ReactElement, type ReactNode } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { toast } from 'sonner'
import { beforeEach, expect, onTestFinished, test, vi } from 'vitest'

import { useRescrapeLibraryMovie, useStartLibraryScan } from '@/api/library'
import { taskKeys, type ScanTask } from '@/api/tasks'
import { LibraryMovieActions } from '@/features/library/movie-actions'
import { LibraryScanButton } from '@/features/library/scan-button'
import { MediaSyncSection } from '@/features/settings/media-sync-section'
import { notifyScanTask } from '@/features/tasks/task-toast'

vi.mock('@/api/library', () => ({
  useRescrapeLibraryMovie: vi.fn(),
  useStartLibraryScan: vi.fn(),
  useLibraryPending: () => ({ data: { count: 0 } }),
  useFailedRescrapeCount: () => ({ data: { count: 3 }, isPending: false }),
  useRescrapeFailedMovies: () => ({ mutate: vi.fn(), isPending: false })
}))
vi.mock('@/features/tasks/use-active-scan', () => ({
  useActiveScan: () => ({ tasks: { isPending: false }, scans: [], active: null })
}))
vi.mock('@/features/tasks/task-events', () => ({
  useTaskConnection: () => ({ status: 'connected' })
}))
vi.mock('sonner', () => ({
  toast: { info: vi.fn(), error: vi.fn(), success: vi.fn() }
}))

beforeEach(() => vi.clearAllMocks())

const queued: ScanTask = {
  id: 254,
  type: 'scan',
  movie_id: 48,
  code: 'PFES-093CH',
  rebuild: true,
  status: 'queued',
  progress: 0,
  created_at: '',
  updated_at: '',
  source: { account_id: 'acc1', directory: { id: 'dir1', name: 'Movies', path: '/Movies' } },
  scan: {
    stage: 'scraping',
    current_path: '/Movies',
    directories_discovered: 0,
    directories_scanned: 0,
    files_scanned: 0,
    video_files: 1,
    matched_files: 1,
    unmatched_files: 0,
    movies: 1,
    removed_files: 0,
    removed_movies: 0,
    metadata_total: 1,
    metadata_completed: 0
  }
}

function elements(node: ReactNode): ReactElement<Record<string, unknown>>[] {
  return Children.toArray(node).flatMap(child => {
    if (!isValidElement<Record<string, unknown>>(child)) return []
    // Expand the presentational scan control so its button is clickable here.
    const children =
      child.type === LibraryScanButton
        ? (child.type as unknown as (props: Record<string, unknown>) => ReactNode)(child.props)
        : (child.props.children as ReactNode)
    return [child, ...elements(children)]
  })
}

function submission(action: 'rescrape' | 'scan' | 'rebuild') {
  const client = new QueryClient()
  onTestFinished(() => client.clear())
  const mutate = vi.fn()
  vi.mocked(useRescrapeLibraryMovie).mockReturnValue({
    mutate,
    isPending: false
  } as unknown as ReturnType<typeof useRescrapeLibraryMovie>)
  vi.mocked(useStartLibraryScan).mockReturnValue({
    mutate,
    isPending: false
  } as unknown as ReturnType<typeof useStartLibraryScan>)
  let tree: ReactNode
  function Capture() {
    tree =
      action === 'rescrape'
        ? LibraryMovieActions({
            movie: {
              id: 48,
              code: queued.code!
            }
          })
        : MediaSyncSection()
    return null
  }
  renderToStaticMarkup(
    <QueryClientProvider client={client}>
      <Capture />
    </QueryClientProvider>
  )
  const event = 'onClick'
  const controls = elements(tree).filter(element => typeof element.props[event] === 'function')
  // The section renders scan, rescrape and rebuild controls in that order.
  ;(controls[action === 'rebuild' ? 2 : 0]!.props[event] as () => void)()
  const callbacks = mutate.mock.calls[0]![1] as { onSuccess: (task: ScanTask) => void }
  return { client, complete: () => callbacks.onSuccess(queued) }
}

for (const action of ['rescrape', 'scan', 'rebuild'] as const) {
  for (const status of ['done', 'failed'] as const) {
    test(`${action}: a late submission callback preserves the latest ${status} notification`, () => {
      const f = submission(action)
      const latest: ScanTask = {
        ...queued,
        status,
        progress: 100,
        error: status === 'failed' ? 'No matching metadata' : undefined,
        can_retry: status === 'failed',
        scan: {
          ...queued.scan,
          metadata_completed: 1,
          metadata_failed: status === 'failed' ? 1 : 0
        }
      }
      // The refresh or SSE event finishes and announces the task before onSuccess runs.
      f.client.setQueryData(taskKeys.all, [latest])
      notifyScanTask(latest)
      vi.clearAllMocks()
      f.complete()
      expect(toast.info).not.toHaveBeenCalled()
      const notify = status === 'failed' ? toast.error : toast.success
      expect(notify).toHaveBeenCalledOnce()
      expect(vi.mocked(notify).mock.calls[0]![1]).toMatchObject({
        id: 'scan:254',
        description: latest.error,
        duration: status === 'failed' ? 8000 : 3000
      })
    })
  }

  test(`${action}: a new task no longer shows an in-progress toast`, () => {
    const f = submission(action)
    f.complete()
    expect(toast.info).not.toHaveBeenCalled()
    expect(toast.success).not.toHaveBeenCalled()
    expect(toast.error).not.toHaveBeenCalled()
  })
}
