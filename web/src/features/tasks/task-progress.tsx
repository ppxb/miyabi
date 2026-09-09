import type { ScanTask } from '@/api/tasks'
import { Progress } from '@/components/ui/progress'

type TaskStage = ScanTask['scan']['stage'] | 'downloading'

const stages: { id: TaskStage; label: string }[] = [
  { id: 'downloading', label: '正在下载' },
  { id: 'scanning', label: '正在扫描文件' },
  { id: 'reconciling', label: '正在核对媒体库' },
  { id: 'scraping', label: '正在刮削元数据' },
  { id: 'artwork', label: '正在写回元数据' },
  { id: 'done', label: '处理完成' }
]

export function TaskProgress({
  current,
  offline = false,
  progress = 0
}: {
  current: TaskStage
  offline?: boolean
  progress?: number
}) {
  const visible = offline ? stages : stages.slice(1)
  const stage = current === 'queued' ? 'scanning' : current
  const index = visible.findIndex(item => item.id === stage)
  // Completion is the final marker. Each preceding stage occupies one interval;
  // stages without a measurable total advance when the next stage starts.
  const value = stage === 'done' ? 100 : ((index + progress / 100) / (visible.length - 1)) * 100

  return (
    <div className="space-y-1">
      <p className="text-xs leading-4 text-muted-foreground">
        {current === 'queued' ? '等待扫描' : visible[index].label}
      </p>
      <Progress
        value={value}
        className="h-1 **:data-[slot=progress-indicator]:bg-emerald-600 dark:**:data-[slot=progress-indicator]:bg-emerald-500"
      />
    </div>
  )
}
