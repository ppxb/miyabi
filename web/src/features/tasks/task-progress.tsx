import { Progress } from '@/components/ui/progress'
import { taskProgressState, type TaskStage } from './task-progress-state'

export function TaskProgress({
  current,
  offline = false,
  progress = 0,
  count,
  label: customLabel
}: {
  current: TaskStage
  offline?: boolean
  progress?: number
  count?: string
  label?: string
}) {
  const { label, value } = taskProgressState(current, offline, progress)

  return (
    <div className="space-y-1">
      <p className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1 text-xs leading-4 text-muted-foreground">
        <span>{customLabel ?? label}</span>
        {count ? <span className="min-w-0 tabular-nums">{count}</span> : null}
      </p>
      <Progress value={value} variant="success" className="h-1" />
    </div>
  )
}
