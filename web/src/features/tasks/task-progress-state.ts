import type { ScanTask } from '@/api/tasks'

export type TaskStage = ScanTask['scan']['stage'] | 'downloading' | 'locating'

const stages: { id: TaskStage; label: string }[] = [
  { id: 'downloading', label: '正在下载' },
  { id: 'scanning', label: '正在扫描文件' },
  { id: 'reconciling', label: '正在核对媒体库' },
  { id: 'scraping', label: '正在刮削元数据' },
  { id: 'artwork', label: '正在写回元数据' },
  { id: 'done', label: '处理完成' }
]

export function taskProgressState(current: TaskStage, offline = false, progress = 0) {
  const visible = offline ? stages : stages.slice(1)
  const stage = current === 'queued' || current === 'locating' ? 'scanning' : current
  const index = visible.findIndex(item => item.id === stage)
  const active = visible[index]
  if (!active) return { label: '等待进度同步', value: null }

  const percent = Number.isFinite(progress) ? Math.min(100, Math.max(0, progress)) : 0
  // Completion is the final marker; each preceding stage occupies one interval.
  const value = stage === 'done' ? 100 : ((index + percent / 100) / (visible.length - 1)) * 100
  const label =
    current === 'locating'
      ? '已下载，等待 115 返回文件信息'
      : current === 'queued'
        ? '等待扫描'
        : active.label
  return { label, value }
}
