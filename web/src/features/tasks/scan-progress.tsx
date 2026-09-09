import { RefreshCwIcon } from 'lucide-react'

import { isTaskActive, type ScanTask } from '@/api/tasks'
import { OverflowTooltip } from '@/components/overflow-tooltip'
import { Button } from '@/components/ui/button'
import { Progress } from '@/components/ui/progress'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { useTaskConnection } from '@/features/tasks/task-events'
import { scanCount, scanStatus } from './scan-status'

export function ScanProgressView({ task }: { task: ScanTask }) {
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
  const count = scanCount(task)
  const summary = `${status}，${count}`
  const value = active && !metadata && !disconnected ? null : task.progress

  return (
    <div className="min-w-0 flex-1 space-y-2">
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
      {task.error ? (
        <OverflowTooltip content={task.error}>
          <p className="truncate text-xs text-destructive">{task.error}</p>
        </OverflowTooltip>
      ) : null}
    </div>
  )
}
