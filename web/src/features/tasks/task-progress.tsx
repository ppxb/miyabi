import { Progress } from '@/components/ui/progress'
import { taskProgressState, type TaskStage } from './task-progress-state'

export function TaskProgress({
  current,
  offline = false,
  progress = 0,
  hint,
  label: customLabel
}: {
  current: TaskStage
  offline?: boolean
  progress?: number
  label?: string
  hint?: string
}) {
  const { label, value } = taskProgressState(current, offline, progress)

  return (
    <div className="space-y-1">
      <p className="text-xs leading-4 text-muted-foreground" title={hint}>
        {customLabel ?? label}
      </p>
      <Progress value={value} variant="success" className="h-1" />
    </div>
  )
}
