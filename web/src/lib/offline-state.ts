import type { OfflineSubmission } from '@/api/offline'

export function isOfflineTaskActive(task: OfflineSubmission) {
  return task.phase === 'downloading' || task.processing
}
