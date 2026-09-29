import { LoaderCircleIcon, RefreshCwIcon, ScanLineIcon } from 'lucide-react'

import { describeApiError } from '@/api/client'
import { useStartLibraryScan } from '@/api/library'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { notifyScanTask, notifyTaskError } from '@/features/tasks/task-toast'

export function LibraryScanButton({
  scanning,
  connected,
  failed,
  onStarted
}: {
  scanning: boolean
  connected: boolean
  failed: boolean
  onStarted: () => void
}) {
  const startScan = useStartLibraryScan()
  const processing = scanning && connected
  const scanLabel = scanning
    ? processing
      ? '正在处理'
      : '等待同步'
    : failed
      ? '重新扫描'
      : '扫描媒体库'

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          className="w-9 px-0 sm:w-auto sm:px-3"
          disabled={scanning || startScan.isPending}
          onClick={() =>
            startScan.mutate(undefined, {
              onSuccess: task => {
                notifyScanTask(task)
                onStarted()
              },
              onError: error => {
                notifyTaskError('scan:submit-error', '无法创建扫描任务', describeApiError(error))
              }
            })
          }
        >
          {processing || startScan.isPending ? (
            <LoaderCircleIcon className="size-4 animate-spin" />
          ) : scanning ? (
            <RefreshCwIcon className="size-4" />
          ) : (
            <ScanLineIcon className="size-4" />
          )}
          <span className="hidden sm:inline">{scanLabel}</span>
        </Button>
      </TooltipTrigger>
      <TooltipContent className="sm:hidden">{scanLabel}</TooltipContent>
    </Tooltip>
  )
}
