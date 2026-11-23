import { RotateCcwIcon, ScanLineIcon } from 'lucide-react'
import { Spinner } from '@/components/ui/spinner'

import { describeApiError } from '@/api/client'
import { useStartLibraryScan } from '@/api/library'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { notifyTaskError, useNotifyScanTask } from '@/features/tasks/task-toast'

export function LibraryScanButton({
  loading,
  scanning,
  rebuilding,
  connected
}: {
  loading: boolean
  scanning: boolean
  rebuilding: boolean
  connected: boolean
}) {
  const startScan = useStartLibraryScan()
  const notifyScan = useNotifyScanTask()
  // Keep the mutation observer mounted while pagination replaces the visible control.
  if (loading) return <Skeleton className="h-8 w-60 rounded-lg" />

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
            disabled={scanning || startScan.isPending}
            onClick={() =>
              startScan.mutate(rebuild, {
                onSuccess: notifyScan,
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
              <Spinner data-icon="inline-start" />
            ) : rebuild ? (
              <RotateCcwIcon data-icon="inline-start" />
            ) : (
              <ScanLineIcon data-icon="inline-start" />
            )}
            {label}
          </Button>
        )
      })}
    </div>
  )
}
