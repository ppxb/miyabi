import type { OfflineSubmission } from '@/api/offline'

export function downloadStatus(task: OfflineSubmission): string {
  switch (task.download_state) {
    case 'queued':
      return '等待 115 开始下载'
    case 'switching':
      return '正在切换磁力'
    case 'submitting':
      return '正在提交下载'
    case 'cancelling':
      return '正在取消下载'
    case 'waiting':
      return '暂时无法处理，等待重试'
    case 'stalled':
      return '暂无进展，继续等待'
    case 'exhausted':
      return '停止换源，继续等待'
    default:
      return '正在下载'
  }
}
