import { isTaskActive, type ScanTask } from '@/api/tasks'

const statusLabels = {
  queued: '等待扫描',
  running: '扫描中',
  done: '处理完成',
  failed: '处理失败'
}

export function scanStatus(task: ScanTask) {
  if (!isTaskActive(task)) return statusLabels[task.status]
  const stage = scanStage(task)
  if (task.scan.metadata_total > 0) {
    if (task.status === 'queued') return '等待刮削'
    return stage === 'artwork' ? '写入元数据' : '刮削中'
  }
  if (task.status === 'running' && stage === 'reconciling') return '核对中'
  return statusLabels[task.status]
}

export function scanStage(task: ScanTask): ScanTask['scan']['stage'] {
  if (!isTaskActive(task)) return task.status === 'done' ? 'done' : task.scan.stage
  if (task.scan.metadata_total > 0) return task.scan.stage === 'artwork' ? 'artwork' : 'scraping'
  return task.scan.stage === 'done' ? 'reconciling' : task.scan.stage
}

export function scanCount(task: ScanTask) {
  return task.scan.metadata_total > 0
    ? `${task.scan.metadata_completed} / ${task.scan.metadata_total} 部`
    : `识别到 ${task.scan.movies} 部`
}
