import { CircleAlertIcon, RefreshCwIcon } from 'lucide-react'

import { isTaskActive, type ScanTask } from '@/api/tasks'
import { OverflowTooltip } from '@/components/overflow-tooltip'
import { Button } from '@/components/ui/button'
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

function scanStatus(task: ScanTask) {
  if (!isTaskActive(task)) return statusLabels[task.status]
  if (task.scan.metadata_total > 0) {
    if (task.status === 'queued') return '等待刮削'
    return task.scan.stage === 'artwork' ? '写入元数据' : '刮削中'
  }
  if (task.status === 'running' && ['reconciling', 'done'].includes(task.scan.stage))
    return '核对中'
  return statusLabels[task.status]
}

export function ScanProgressView({ task, compact = false }: { task: ScanTask; compact?: boolean }) {
  const connection = useTaskConnection()
  const active = isTaskActive(task)
  const disconnected = active && connection.status === 'disconnected'
  const scan = task.scan
  const metadata = scan.metadata_total > 0
  const status = disconnected
    ? '进度已断开'
    : active && connection.status === 'connecting'
      ? '连接进度中'
      : scanStatus(task)
  const count = metadata
    ? `${scan.metadata_completed} / ${scan.metadata_total} 部`
    : `识别到 ${scan.movies} 部`
  const summary = `${status}，${count}`
  const value = active && !metadata && !disconnected ? null : task.progress

  return (
    <div className="min-w-0 flex-1">
      {compact ? (
        <Tooltip>
          <TooltipTrigger asChild>
            {disconnected ? (
              <Button
                variant="ghost"
                size="icon"
                className="h-9 w-6 sm:hidden"
                onClick={connection.reconnect}
              >
                <RefreshCwIcon />
              </Button>
            ) : (
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
            )}
          </TooltipTrigger>
          <TooltipContent>
            {disconnected ? '进度连接已中断，点击重新连接' : task.error || summary}
          </TooltipContent>
        </Tooltip>
      ) : null}
      <div className={cn('space-y-2', compact && 'hidden sm:block')}>
        <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
          {disconnected ? (
            <Button variant="link" size="xs" className="h-5 px-0" onClick={connection.reconnect}>
              <RefreshCwIcon />
              重连进度
            </Button>
          ) : (
            <Tooltip>
              <TooltipTrigger asChild>
                <p className="shrink-0 text-xs">{status}</p>
              </TooltipTrigger>
              <TooltipContent>{task.error || summary}</TooltipContent>
            </Tooltip>
          )}
          <Progress
            value={value}
            className="order-last h-1 w-full **:data-[slot=progress-indicator]:bg-emerald-600 sm:order-0 sm:w-auto sm:flex-1 dark:**:data-[slot=progress-indicator]:bg-emerald-500"
          />
          <span className="ml-auto shrink-0 text-xs text-muted-foreground tabular-nums sm:ml-0">
            {count}
          </span>
        </div>
        {task.error && !compact ? (
          <OverflowTooltip content={task.error}>
            <p className="truncate text-xs text-destructive">{task.error}</p>
          </OverflowTooltip>
        ) : null}
      </div>
    </div>
  )
}
