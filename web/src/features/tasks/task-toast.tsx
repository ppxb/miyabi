import { useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'

import type { OfflineSubmission } from '@/api/offline'
import { Spinner } from '@/components/ui/spinner'
import { isScanTask, isTaskActive, taskKeys, type ScanTask, type Task } from '@/api/tasks'
import { isOfflineTaskActive } from '@/api/offline'
import { scanStage, scanStatus } from './scan-status'
import { TaskProgress } from './task-progress'
import { TaskToastActions } from './task-toast-actions'
import { offlineToastID, scanToastID } from './task-notification-diff'
import { downloadStatus } from './download-status'

type TaskToastOptions = {
  waiting?: boolean
  onDismiss?: () => void
}

function taskToastOptions(
  id: string,
  active: boolean,
  options: TaskToastOptions = {},
  retryTaskID?: number,
  cancelTaskID?: number
) {
  return {
    id,
    duration: active ? Infinity : 8000,
    closeButton: false,
    onDismiss: options.onDismiss,
    action: <TaskToastActions id={id} retryTaskID={retryTaskID} cancelTaskID={cancelTaskID} />
  }
}

export function notifyTaskError(id: string, title: string, description: string) {
  toast.error(title, { ...taskToastOptions(id, false), description })
}

export function useNotifyScanTask() {
  const queryClient = useQueryClient()
  return (task: ScanTask) => {
    // Mutation callbacks may run after a refresh or SSE has already finished the task.
    const latest = queryClient
      .getQueryData<Task[]>(taskKeys.all)
      ?.find((item): item is ScanTask => isScanTask(item) && item.id === task.id)
    notifyScanTask(latest ?? task)
  }
}

export function notifyScanTask(task: ScanTask, options: TaskToastOptions = {}) {
  // In-progress scans use the library ring indicator instead of a toast;
  // only terminal results (completed/failed) surface a notification.
  if (isTaskActive(task)) return
  const action = task.rebuild ? '重建' : '同步'
  const title = task.movie_id ? `${task.code} 重新刮削` : `媒体库${action}`
  const props = taskToastOptions(
    scanToastID(task.id),
    false,
    options,
    task.can_retry ? task.id : undefined
  )
  if (task.status === 'failed') {
    toast.error(task.scan.metadata_failed && !task.movie_id ? `${title}结束` : `${title}失败`, {
      ...props,
      description:
        task.scan.metadata_failed && !task.movie_id
          ? `${task.scan.metadata_failed} 部影片失败`
          : task.error
    })
  } else {
    toast.success(`${title}完成`, {
      ...props,
      // sonner merges updates into a same-id toast; clear a prior failure's
      // description (e.g. after a retry succeeds).
      description: undefined
    })
  }
}

export function notifyOfflineTask(
  task: OfflineSubmission,
  options: TaskToastOptions & { scan?: ScanTask } = {}
) {
  const active = isOfflineTaskActive(task)
  const id = offlineToastID(task.task_id)
  const props = taskToastOptions(
    id,
    active,
    options,
    options.scan?.can_retry ? options.scan.id : undefined,
    task.can_cancel ? task.task_id : undefined
  )
  if (active) {
    const scan = options.scan
    toast.info(task.code, {
      ...props,
      icon:
        options.waiting || (task.phase !== 'downloading' && scan?.paused) ? undefined : <Spinner />,
      description: options.waiting ? (
        '等待进度同步'
      ) : (
        <TaskProgress
          offline
          hint={task.error ?? task.switch_reason}
          label={
            task.phase === 'downloading'
              ? downloadStatus(task)
              : scan
                ? scanStatus(scan)
                : undefined
          }
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
              : scan && scan.scan.metadata_total > 0
                ? scan.progress
                : undefined
          }
        />
      )
    })
  } else if (task.status === 'cancelled') {
    toast.info(task.code, { ...props, description: '已取消下载' })
  } else if (task.phase === 'in_library') {
    const notify = task.error ? toast.warning : toast.success
    notify(task.code, {
      ...props,
      description: task.error ? `元数据处理失败：${task.error}` : '下载与入库处理已完成'
    })
  } else if (task.error || task.status === 'failed') {
    toast.error(task.code, { ...props, description: task.error ?? '处理失败，请重试。' })
  } else {
    toast.warning(task.code, {
      ...props,
      description:
        task.phase === 'downloaded'
          ? '视频已下载，但尚未识别为对应影片，请在 115 检查文件名和大小后同步媒体库。'
          : '当前媒体目录内未找到该任务的视频文件。'
    })
  }
}
