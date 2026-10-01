import { LoaderCircleIcon, RefreshCwIcon, RotateCcwIcon, ScanLineIcon } from 'lucide-react'

import { describeApiError } from '@/api/client'
import { useStartLibraryScan } from '@/api/library'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { notifyScanTask, notifyTaskError } from '@/features/tasks/task-toast'

export function LibraryScanButton({
  loading,
  available,
  scanning,
  rebuilding,
  connected,
  failed,
  onStarted
}: {
  loading: boolean
  available: boolean
  scanning: boolean
  rebuilding: boolean
  connected: boolean
  failed: boolean
  onStarted: () => void
}) {
  const startScan = useStartLibraryScan()
  // Keep the mutation observer mounted while pagination replaces the visible control.
  if (loading) return <Skeleton className="h-9 w-20 rounded-4xl sm:w-60" />
  if (!available) return null

  const processing = scanning && connected
  const scanLabel = scanning
    ? processing
      ? '正在处理'
      : '等待同步'
    : failed
      ? '重新扫描'
      : '扫描媒体库'

  return (
    <div className="flex items-center gap-2">
      {[false, true].map(rebuild => {
        const label = rebuild ? (scanning && rebuilding ? '正在重建' : '重建媒体库') : scanLabel
        const busy =
          (processing && rebuilding === rebuild) ||
          (startScan.isPending && startScan.variables === rebuild)
        return (
          <Button
            key={String(rebuild)}
            variant={rebuild ? 'outline' : 'default'}
            className="w-9 px-0 sm:w-auto sm:px-3"
            disabled={scanning || startScan.isPending}
            onClick={() =>
              startScan.mutate(rebuild, {
                onSuccess: task => {
                  notifyScanTask(task)
                  onStarted()
                },
                onError: error => {
                  notifyTaskError(
                    'scan:submit-error',
                    rebuild ? '无法创建重建任务' : '无法创建扫描任务',
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
            ) : scanning ? (
              <RefreshCwIcon className="size-4" />
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
