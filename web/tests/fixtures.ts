import type { OfflineSubmission } from '@/api/offline'

export function offlineSubmission(overrides: Partial<OfflineSubmission> = {}): OfflineSubmission {
  return {
    task_id: 1,
    code: 'ABP-001',
    javdb_id: 'one',
    account_id: 'acc1',
    directory_id: 'dir1',
    hash: 'fixture-hash',
    status: 'done',
    phase: 'in_library',
    processing: false,
    progress: 100,
    ...overrides
  }
}
