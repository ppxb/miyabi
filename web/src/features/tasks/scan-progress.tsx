import { CircleAlertIcon } from 'lucide-react'

import { isTaskActive, type ScanTask } from '@/api/tasks'
import { OverflowTooltip } from '@/components/overflow-tooltip'
import { Progress } from '@/components/ui/progress'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { useTaskConnection } from '@/features/tasks/task-events'
import { cn } from '@/lib/utils'

const statusLabels = {
  queued: '等待扫描',
  running: '扫描中',
  done: '处理完成',
  failed: '处理失败'
}

function scanStatus(task: ScanTask, reconnecting: boolean) {
  if (!isTaskActive(task)) return statusLabels[task.status]
  if (reconnecting) return '进度重连中'
  if (task.scan.metadata_total > 0) {
    if (task.status === 'queued') return '等待刮削'
    return task.scan.stage === 'artwork' ? '写入元数据' : '刮削中'
  }
  if (task.status === 'running' && ['reconciling', 'done'].includes(task.scan.stage))
    return '核对中'
  return statusLabels[task.status]
}

export function ScanProgressView({
  task,
  compactOnMobile = false
}: {
  task: ScanTask
  compactOnMobile?: boolean
}) {
  const connection = useTaskConnection()
  const active = isTaskActive(task)
  const scan = task.scan
  const metadata = scan.metadata_total > 0
  const status = scanStatus(task, connection === 'reconnecting')
  const count = metadata
    ? `${scan.metadata_completed} / ${scan.metadata_total} 部`
    : `识别到 ${scan.movies} 部`
  const summary = `${status}，${count}`
  const value = active && !metadata ? null : task.progress

  return (
    <div className="min-w-0">
      {compactOnMobile ? (
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="flex h-9 items-center rounded outline-ring sm:hidden">
              {task.status === 'failed' ? (
                <CircleAlertIcon className="size-6 text-destructive" />
              ) : (
                <Progress
                  variant="circular"
                  value={value}

                  className="text-emerald-600 dark:text-emerald-500"
                />
              )}
            </span>
          </TooltipTrigger>
          <TooltipContent>{task.error || summary}</TooltipContent>
        </Tooltip>
      ) : null}
      <div className={cn('space-y-2', compactOnMobile && 'hidden sm:block')}>
        <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
          <p className="shrink-0 text-xs">{status}</p>
          <Progress
            value={value}

            className="order-last h-1 w-full **:data-[slot=progress-indicator]:bg-emerald-600 sm:order-0 sm:w-auto sm:flex-1 dark:**:data-[slot=progress-indicator]:bg-emerald-500"
          />
          <span className="ml-auto shrink-0 text-xs text-muted-foreground tabular-nums sm:ml-0">
            {count}
          </span>
        </div>
        {task.error ? (
          <OverflowTooltip content={task.error}>
            <p className="truncate text-xs text-destructive">{task.error}</p>
          </OverflowTooltip>
        ) : null}
      </div>
    </div>
  )
}
