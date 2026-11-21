import { isTaskActive, type ScanTask } from '@/api/tasks'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'

export function LibraryProgress({ task }: { task?: ScanTask }) {
  if (!task || !isTaskActive(task)) return null
  const { scan } = task
  const total = scan.metadata_total
  const done = scan.metadata_completed
  const failed = scan.metadata_failed ?? 0
  const percent =
    total > 0
      ? ((done + failed) / total) * 100
      : scan.directories_discovered > 0
        ? (scan.directories_scanned / scan.directories_discovered) * 100
        : task.progress
  const bounded = Math.max(0, Math.min(100, Math.round(percent)))
  const label =
    total > 0
      ? `刮削 ${done}/${total}${failed > 0 ? ` · 失败 ${failed}` : ''}`
      : `扫描中：${scan.movies} 部影片 / ${scan.files_scanned} 个文件`
  const radius = 18
  const circumference = 2 * Math.PI * radius
  const offset = circumference * (1 - bounded / 100)

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <div
            role="status"
            aria-label={label}
            className="relative inline-flex size-11 shrink-0 items-center justify-center"
          />
        }
      >
          <svg
            viewBox="0 0 44 44"
            className="size-11 -rotate-90"
            shapeRendering="geometricPrecision"
          >
            <circle
              cx="22"
              cy="22"
              r={radius}
              fill="none"
              strokeWidth="4"
              className="stroke-muted"
            />
            <circle
              cx="22"
              cy="22"
              r={radius}
              fill="none"
              strokeWidth="4"
              strokeLinecap="round"
              strokeDasharray={circumference}
              strokeDashoffset={offset}
              className="stroke-success transition-[stroke-dashoffset] duration-500"
            />
          </svg>
          <span className="absolute text-[10px] font-medium tabular-nums">{bounded}%</span>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  )
}
