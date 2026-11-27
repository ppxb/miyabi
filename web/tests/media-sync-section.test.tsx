import { Children, isValidElement, type ReactElement, type ReactNode } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { toast } from 'sonner'
import { beforeEach, expect, test, vi } from 'vitest'

import { LibraryScanButton } from '@/features/library/scan-button'
import { MediaSyncSection } from '@/features/settings/media-sync-section'
import { notifyTaskError } from '@/features/tasks/task-toast'

const state = vi.hoisted(() => ({
  failedCount: 3,
  scanPending: false,
  scanMutate: vi.fn(),
  rescrapeMutate: vi.fn(),
  scans: [] as Array<Record<string, unknown>>,
  scanActive: null as Record<string, unknown> | null
}))

vi.mock('@/api/library', () => ({
  useLibraryPending: () => ({ data: { count: 1 } }),
  useFailedRescrapeCount: () => ({ data: { count: state.failedCount }, isPending: false }),
  useRescrapeFailedMovies: () => ({ mutate: state.rescrapeMutate, isPending: false }),
  useStartLibraryScan: () => ({ mutate: state.scanMutate, isPending: state.scanPending })
}))
vi.mock('@/features/tasks/use-active-scan', () => ({
  useActiveScan: () => ({
    tasks: { isPending: false },
    scans: state.scans,
    active: state.scanActive
  })
}))
vi.mock('@/features/tasks/task-events', () => ({
  useTaskConnection: () => ({ status: 'connected' })
}))
vi.mock('@/features/tasks/task-toast', () => ({
  useNotifyScanTask: () => vi.fn(),
  notifyTaskError: vi.fn()
}))
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

beforeEach(() => {
  vi.clearAllMocks()
  state.failedCount = 3
  state.scanPending = false
  state.scans = []
  state.scanActive = null
})

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

function buttons(): ReactElement<Record<string, unknown>>[] {
  return elements(MediaSyncSection()).filter(element => typeof element.props.onClick === 'function')
}

function labelOf(button: ReactElement<Record<string, unknown>>): ReactNode {
  return Children.toArray(button.props.children as ReactNode).at(-1)
}

function button(label: string): ReactElement<Record<string, unknown>> {
  const found = buttons().find(item => labelOf(item) === label)
  if (!found) throw new Error(`button ${label} not rendered`)
  return found
}

test('renders only the failed movies that can be rescraped', () => {
  expect(renderToStaticMarkup(MediaSyncSection())).toContain('共 3 部刮削失败，可批量提交')
  state.failedCount = 0
  expect(renderToStaticMarkup(MediaSyncSection())).toContain('当前没有可重新刮削的失败影片')
  expect(button('重新刮削失败影片').props.disabled).toBe(true)
})

test('bulk rescrape reports created, reused and skipped movies separately', () => {
  ;(button('重新刮削失败影片').props.onClick as () => void)()
  expect(state.rescrapeMutate).toHaveBeenCalledOnce()
  const callbacks = state.rescrapeMutate.mock.calls[0]![1] as {
    onSuccess: (result: { created: number; reused: number; skipped: number }) => void
  }
  callbacks.onSuccess({ created: 2, reused: 1, skipped: 1 })
  expect(toast.success).toHaveBeenCalledWith('已提交 2 部，1 部已在队列中，1 部无法匹配来源已跳过')
})

test('both scan controls disable while a submission is pending', () => {
  state.scanPending = true
  for (const label of ['扫描并修复', '重建媒体库']) {
    expect(button(label).props.disabled).toBe(true)
  }
})

test('a running bulk rescrape disables every action without showing a scan spinner', () => {
  const bulk = { type: 'scan', status: 'running', rebuild: false, bulk_rescrape: true }
  state.scans = [bulk]
  state.scanActive = bulk
  expect(button('扫描并修复').props.disabled).toBe(true)
  expect(button('重建媒体库').props.disabled).toBe(true)
  expect(button('正在重新刮削').props.disabled).toBe(true)
})

test('a running library scan disables the bulk rescrape action', () => {
  const scan = { type: 'scan', status: 'running', rebuild: false }
  state.scans = [scan]
  state.scanActive = scan
  expect(button('正在扫描').props.disabled).toBe(true)
  expect(button('重建媒体库').props.disabled).toBe(true)
  expect(button('重新刮削失败影片').props.disabled).toBe(true)
})

test('scan submission reports its own failure message', () => {
  ;(button('扫描并修复').props.onClick as () => void)()
  expect(state.scanMutate.mock.calls[0]![0]).toBe(false)
  const callbacks = state.scanMutate.mock.calls[0]![1] as { onError: (error: Error) => void }
  callbacks.onError(new Error('request failed'))
  expect(notifyTaskError).toHaveBeenCalledWith(
    'scan:submit-error',
    '无法创建扫描任务',
    '请检查后端服务和网络后重试。'
  )
})
