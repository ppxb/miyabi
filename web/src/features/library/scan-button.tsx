import { LoaderCircleIcon, RotateCcwIcon, ScanLineIcon } from 'lucide-react'

import { describeApiError } from '@/api/client'
import { useStartLibraryScan } from '@/api/library'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { notifyTaskError, useNotifyScanTask } from '@/features/tasks/task-toast'

export function LibraryScanButton({
  loading,
  available,
  scanning,
  rebuilding,
  connected,
  onStarted
}: {
  loading: boolean
  available: boolean
  scanning: boolean
  rebuilding: boolean
  connected: boolean
  onStarted: () => void
}) {
  const startScan = useStartLibraryScan()
  const notifyScan = useNotifyScanTask()
  // Keep the mutation observer mounted while pagination replaces the visible control.
  if (loading) return <Skeleton className="h-9 w-20 rounded-4xl sm:w-60" />
  if (!available) return null

  return (
    <div className="flex items-center gap-2">
      {[false, true].map(rebuild => {
        const active = scanning && rebuilding === rebuild
        const activityLabel = connected ? (rebuild ? '正在重建' : '正在同步') : '连接中'
        const label = active ? activityLabel : rebuild ? '重建媒体库' : '同步媒体库'
        const busy =
          (active && connected) || (startScan.isPending && startScan.variables === rebuild)
        return (
          <Button
            key={String(rebuild)}
            variant={rebuild ? 'outline' : 'default'}
            className="w-9 px-0 sm:w-auto sm:px-3"
            disabled={scanning || startScan.isPending}
            onClick={() =>
              startScan.mutate(rebuild, {
                onSuccess: task => {
                  notifyScan(task)
                  onStarted()
                },
                onError: error => {
                  notifyTaskError(
                    'scan:submit-error',
                    rebuild ? '无法创建重建任务' : '无法创建同步任务',
                    describeApiError(error)
                  )
                }
              })
            }
          >
            {busy ? (
              <LoaderCircleIcon className="size-4 animate-spin" />
            ) : rebuild ? (
              <RotateCcwIcon className="size-4" />
            ) : (
              <ScanLineIcon className="size-4" />
            )}
            <span className="hidden sm:inline">{label}</span>
          </Button>
        )
      })}
    </div>
  )
}
