import { Progress } from '@/components/ui/progress'
import { taskProgressState, type TaskStage } from './task-progress-state'

export function TaskProgress({
  current,
  offline = false,
  progress = 0,
  count
}: {
  current: TaskStage
  offline?: boolean
  progress?: number
  count?: string
}) {
  const { label, value } = taskProgressState(current, offline, progress)

  return (
    <div className="space-y-1">
      <p className="flex items-center justify-between gap-3 text-xs leading-4 text-muted-foreground">
        <span>{label}</span>
        {count ? <span className="shrink-0 tabular-nums">{count}</span> : null}
      </p>
      <Progress value={value} variant="success" className="h-1" />
    </div>
  )
}
