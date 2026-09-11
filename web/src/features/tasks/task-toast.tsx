import { LoaderCircleIcon } from 'lucide-react'
import { toast } from 'sonner'

import type { OfflineSubmission } from '@/api/offline'
import { isTaskActive, type ScanTask } from '@/api/tasks'
import { isOfflineTaskActive } from '@/lib/offline-state'
import { useUIStore } from '@/stores/ui'
import { scanStage } from './scan-status'
import { TaskProgress } from './task-progress'
import { TaskToastActions } from './task-toast-actions'

type TaskToastOptions = {
  waiting?: boolean
  onDismiss?: () => void
}

export const scanToastID = (id: number) => `scan:${id}`
export const offlineToastID = (id: number) => `offline:${id}`

function taskToastOptions(id: string, active: boolean, options: TaskToastOptions = {}) {
  return {
    id,
    duration: active ? Infinity : 8000,
    dismissible: true,
    closeButton: false,
    icon: undefined,
    onDismiss: options.onDismiss,
    action: <TaskToastActions id={id} />
  }
}

export function notifyTaskError(id: string, title: string, description: string) {
  toast.error(title, { ...taskToastOptions(id, false), description })
}

export function notifyScanTask(task: ScanTask, options: TaskToastOptions = {}) {
  const active = isTaskActive(task)
  const props = taskToastOptions(scanToastID(task.id), active, options)
  if (active) {
    // Sonner's loading type hides the close button; long tasks remain dismissible.
    toast.info(options.waiting ? '扫描进度等待同步' : '正在处理媒体库', {
      ...props,
      icon: options.waiting ? undefined : <LoaderCircleIcon className="size-4 animate-spin" />,
      description: (
        <TaskProgress
          current={scanStage(task)}
          progress={task.scan.stage === 'artwork' ? task.progress : undefined}
        />
      )
    })
  } else if (task.status === 'failed') {
    toast.error('媒体库处理失败', { ...props, description: task.error })
  } else {
    toast.success('媒体库处理完成', {
      ...props,
      description: `识别 ${task.scan.movies} 部影片${task.scan.metadata_total > 0 ? ` · 元数据 ${task.scan.metadata_completed} 部` : ''}`
    })
  }
}

export function notifyOfflineTask(
  task: OfflineSubmission,
  options: TaskToastOptions & { scan?: ScanTask } = {}
) {
  const active = isOfflineTaskActive(task)
  const id = offlineToastID(task.task_id)
  const props = taskToastOptions(id, active, options)
  const libraryID = task.phase === 'in_library' ? task.library_id : undefined
  const actions = (
    <TaskToastActions
      id={id}
      onPlay={
        libraryID === undefined ? undefined : () => useUIStore.getState().openPlayer(libraryID)
      }
    />
  )
  if (active) {
    const scan = options.scan
    toast.info(task.code, {
      ...props,
      action: actions,
      icon: options.waiting ? undefined : <LoaderCircleIcon className="size-4 animate-spin" />,
      description: options.waiting ? (
        '等待进度同步'
      ) : (
        <TaskProgress
          offline
          current={
            task.phase === 'downloading'
              ? 'downloading'
              : scan
                ? scanStage(scan)
                : task.scan_task_id
                  ? 'queued'
                  : 'locating'
          }
          progress={
            task.phase === 'downloading'
              ? task.progress
              : scan?.scan.stage === 'artwork'
                ? scan.progress
                : undefined
          }
        />
      )
    })
  } else if (task.phase === 'in_library') {
    const notify = task.error ? toast.warning : toast.success
    notify(task.code, {
      ...props,
      description: task.error ? `元数据处理失败：${task.error}` : '下载与入库处理已完成',
      action: actions
    })
  } else if (task.error || task.status === 'failed') {
    toast.error(task.code, { ...props, description: task.error ?? '处理失败，请重试。' })
  } else {
    toast.warning(task.code, {
      ...props,
      description:
        task.phase === 'downloaded'
          ? '视频已下载，但尚未识别为对应影片，请在 115 检查文件名和大小后重新扫描。'
          : '当前媒体目录内未找到该任务的视频文件。'
    })
  }
}
