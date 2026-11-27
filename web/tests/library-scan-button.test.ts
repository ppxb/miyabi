import { Children, createElement, type ReactNode } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { expect, test, vi } from 'vitest'

import { useLibraryMovies } from '@/api/library'
import { useTasks } from '@/api/tasks'
import { LibraryPage } from '@/features/library/page'
import { LibraryScanButton } from '@/features/library/scan-button'

vi.mock('@/api/library', () => ({
  LIBRARY_PAGE_SIZE: 20,
  useLibraryMovies: vi.fn(),
  useRecentMovies: () => ({ data: { movies: [], total: 0 }, isPending: false }),
  useTopRatedMovies: () => ({ data: [] })
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

function label(button: { props: Record<string, unknown> }): ReactNode {
  return Children.toArray(button.props.children as ReactNode).at(-1)
}

const base = {
  loading: false,
  scanning: false,
  active: false,
  connected: true,
  pending: false,
  blocked: false,
  onSubmit: () => {}
}

test('library page header shows the multi-source description without scan controls', () => {
  vi.mocked(useTasks).mockReturnValue({ data: [] } as unknown as ReturnType<typeof useTasks>)
  vi.mocked(useLibraryMovies).mockReturnValue({
    isSuccess: true,
    data: { movies: [], total: 0 }
  } as unknown as ReturnType<typeof useLibraryMovies>)
  const html = renderToStaticMarkup(createElement(LibraryPage))
  expect(html).toContain('最近加入、正在观看与高分影片')
  expect(html).not.toContain('扫描并修复')
})

test('the loading placeholder renders without a scan action', () => {
  const node = LibraryScanButton({ ...base, rebuild: false, loading: true })!
  expect(node.props.onClick).toBeUndefined()
})

test('scan and rebuild controls submit their own modes', () => {
  const submit = vi.fn()
  const scan = LibraryScanButton({ ...base, rebuild: false, onSubmit: submit })!
  const rebuild = LibraryScanButton({ ...base, rebuild: true, onSubmit: submit })!
  expect(label(scan)).toBe('扫描并修复')
  expect(label(rebuild)).toBe('重建媒体库')
  ;(scan.props.onClick as () => void)()
  ;(rebuild.props.onClick as () => void)()
  expect(submit.mock.calls).toEqual([[false], [true]])
})

test('either submission and any running scan disable both controls', () => {
  for (const rebuild of [false, true]) {
    expect(LibraryScanButton({ ...base, rebuild, pending: true })!.props.disabled).toBe(true)
    expect(LibraryScanButton({ ...base, rebuild, scanning: true })!.props.disabled).toBe(true)
    expect(LibraryScanButton({ ...base, rebuild })!.props.disabled).toBe(false)
  }
})

test('a sibling action disables the control without a scan spinner', () => {
  for (const rebuild of [false, true]) {
    const node = LibraryScanButton({ ...base, rebuild, blocked: true })!
    expect(node.props.disabled).toBe(true)
    expect(label(node)).toBe(rebuild ? '重建媒体库' : '扫描并修复')
  }
})

test('active scans show their own progress labels', () => {
  const props = { ...base, scanning: true, active: true }
  expect(label(LibraryScanButton({ ...props, rebuild: false })!)).toBe('正在扫描')
  expect(label(LibraryScanButton({ ...props, rebuild: true })!)).toBe('正在重建')
  expect(label(LibraryScanButton({ ...props, rebuild: false, connected: false })!)).toBe('连接中')
})
