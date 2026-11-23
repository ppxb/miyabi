import type { ScanTask } from '@/api/tasks'
import { clamp } from '@/lib/math'

export type TaskStage = ScanTask['scan']['stage'] | 'downloading' | 'locating'

// Stage order drives the bar; the caller always supplies its own status text.
const stages: TaskStage[] = [
  'downloading',
  'scanning',
  'reconciling',
  'scraping',
  'artwork',
  'done'
]

export function taskProgressState(current: TaskStage, offline = false, progress = 0) {
  const visible = offline ? stages : stages.slice(1)
  const stage = current === 'queued' || current === 'locating' ? 'scanning' : current
  const index = visible.indexOf(stage)
  if (index < 0) return { label: '等待进度同步', value: null }

  const percent = Number.isFinite(progress) ? clamp(progress, 0, 100) : 0
  // Metadata and artwork interleave for each movie. Their shared interval uses
  // the completed-movie count, so starting the next movie cannot move it back.
  const metadata = stage === 'scraping' || stage === 'artwork'
  const start = stage === 'artwork' ? index - 1 : index
  const value =
    stage === 'done'
      ? 100
      : ((start + (percent / 100) * (metadata ? 2 : 1)) / (visible.length - 1)) * 100
  const label =
    current === 'locating'
      ? '已下载，等待 115 返回文件信息'
      : current === 'queued'
        ? '等待扫描'
        : undefined
  return { label, value }
}
