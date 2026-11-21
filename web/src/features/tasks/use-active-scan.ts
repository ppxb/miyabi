import { isScanTask, isTaskActive, useTasks } from '@/api/tasks'

/** Tasks query plus the library scan tasks and the currently active one. */
export function useActiveScan() {
  const tasks = useTasks()
  const scans = tasks.data?.filter(isScanTask)
  const active = scans?.find(isTaskActive)
  return { tasks, scans, active }
}
