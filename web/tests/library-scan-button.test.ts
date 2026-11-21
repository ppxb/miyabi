import { Children, createElement, isValidElement, type ReactElement, type ReactNode } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { expect, test, vi } from 'vitest'

import { useLibraryMovies, useStartLibraryScan } from '@/api/library'
import { useTasks } from '@/api/tasks'
import { LibraryPage } from '@/features/library/page'
import { LibraryScanButton } from '@/features/library/scan-button'
import { notifyScanTask, notifyTaskError } from '@/features/tasks/task-toast'

vi.mock('@/api/library', () => ({
  LIBRARY_PAGE_SIZE: 20,
  useLibraryMovies: vi.fn(),
  useRecentMovies: () => ({ data: { movies: [], total: 0 }, isPending: false }),
  useTopRatedMovies: () => ({ data: [] }),
  useStartLibraryScan: vi.fn()
}))
vi.mock('@/api/tasks', () => ({
  useTasks: vi.fn(),
  isScanTask: () => true,
  isTaskActive: () => false
}))
vi.mock('@/features/tasks/task-events', () => ({
  useTaskConnection: () => ({ status: 'connected' })
}))
vi.mock('@/api/history', () => ({
  HISTORY_PAGE_SIZE: 20,
  useHistory: () => ({ data: { items: [] }, isPending: false, isError: false, isFetching: false })
}))
vi.mock('@/features/tasks/task-toast', () => {
  const notifyScanTask = vi.fn()
  return { notifyScanTask, useNotifyScanTask: () => notifyScanTask, notifyTaskError: vi.fn() }
})

function elements(node: ReactNode): ReactElement<Record<string, unknown>>[] {
  return Children.toArray(node).filter(isValidElement) as ReactElement<Record<string, unknown>>[]
}

test('library page header shows the multi-source description without scan controls', () => {
  vi.mocked(useTasks).mockReturnValue({ data: [] } as unknown as ReturnType<typeof useTasks>)
  vi.mocked(useLibraryMovies).mockReturnValue({
    isSuccess: true,
    data: { movies: [], total: 0 }
  } as unknown as ReturnType<typeof useLibraryMovies>)
  const html = renderToStaticMarkup(createElement(LibraryPage))
  expect(html).toContain('最近加入、正在观看与高分影片')
  expect(html).not.toContain('同步媒体库')
})

test('loading and hidden controls still retain the scan mutation hook', () => {
  vi.mocked(useStartLibraryScan).mockReturnValue({ isPending: true } as ReturnType<
    typeof useStartLibraryScan
  >)
  const props = {
    scanning: false,
    rebuilding: false,
    connected: true,
    onStarted: vi.fn()
  }
  LibraryScanButton({ ...props, loading: true, available: false })
  expect(useStartLibraryScan).toHaveBeenCalledTimes(1)
  expect(LibraryScanButton({ ...props, loading: false, available: false })).toBeNull()
  expect(useStartLibraryScan).toHaveBeenCalledTimes(2)
})

test('scan submission keeps success navigation and failure notification', () => {
  const mutate = vi.fn()
  vi.mocked(useStartLibraryScan).mockReturnValue({
    isPending: false,
    mutate
  } as unknown as ReturnType<typeof useStartLibraryScan>)
  const onStarted = vi.fn()
  const tree = LibraryScanButton({
    loading: false,
    available: true,
    scanning: false,
    rebuilding: false,
    connected: true,
    onStarted
  })!
  const button = elements(tree.props.children)[0]!
  ;(button.props.onClick as () => void)()
  expect(mutate.mock.calls[0]![0]).toBe(false)
  const callbacks = mutate.mock.calls[0]![1]
  const task = { id: 123 }
  callbacks.onSuccess(task)
  expect(notifyScanTask).toHaveBeenCalledWith(task)
  expect(onStarted).toHaveBeenCalledOnce()
  callbacks.onError(new Error('request failed'))
  expect(notifyTaskError).toHaveBeenCalledWith(
    'scan:submit-error',
    '无法创建同步任务',
    '请检查后端服务和网络后重试。'
  )
})

test('rebuild control submits a full rebuild and both controls disable during processing', () => {
  const mutate = vi.fn()
  vi.mocked(useStartLibraryScan).mockReturnValue({
    isPending: false,
    mutate
  } as unknown as ReturnType<typeof useStartLibraryScan>)
  const props = {
    loading: false,
    available: true,
    scanning: false,
    rebuilding: false,
    connected: true,
    onStarted: vi.fn()
  }
  const buttons = (tree: ReturnType<typeof LibraryScanButton>) => elements(tree!.props.children)
  const controls = buttons(LibraryScanButton(props))
  expect(
    controls.map(button => Children.toArray(button.props.children as ReactNode).at(-1))
  ).toEqual(['同步媒体库', '重建媒体库'])
  ;(controls[1]!.props.onClick as () => void)()
  expect(mutate.mock.calls[0]![0]).toBe(true)
  for (const button of buttons(LibraryScanButton({ ...props, scanning: true, rebuilding: true }))) {
    expect(button.props.disabled).toBe(true)
  }
})
