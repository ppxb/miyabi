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
  const status =
    active && connection === 'reconnecting'
      ? '进度重连中'
      : active && metadata
        ? task.status === 'queued'
          ? '等待刮削'
          : scan.stage === 'artwork'
            ? '写入元数据'
            : '刮削中'
        : task.status === 'running' && (scan.stage === 'reconciling' || scan.stage === 'done')
          ? '核对中'
          : statusLabels[task.status]
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
            <span
              tabIndex={0}
              aria-label={task.error || summary}
              className="flex h-9 items-center rounded outline-ring sm:hidden"
            >
              {task.status === 'failed' ? (
                <CircleAlertIcon className="size-6 text-destructive" aria-hidden="true" />
              ) : (
                <Progress
                  variant="circular"
                  value={value}
                  aria-label="媒体库处理进度"
                  aria-valuetext={summary}
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
          <p role="status" className="shrink-0 text-xs">
            {status}
          </p>
          <Progress
            value={value}
            aria-label="媒体库处理进度"
            aria-valuetext={status}
            className="order-last h-1 w-full **:data-[slot=progress-indicator]:bg-emerald-600 sm:order-0 sm:w-auto sm:flex-1 dark:**:data-[slot=progress-indicator]:bg-emerald-500"
          />
          <span className="ml-auto shrink-0 text-xs text-muted-foreground tabular-nums sm:ml-0">
            {count}
          </span>
        </div>
        {task.error ? (
          <OverflowTooltip content={task.error}>
            <p role="alert" className="truncate text-xs text-destructive">
              {task.error}
            </p>
          </OverflowTooltip>
        ) : null}
      </div>
    </div>
  )
}
