import { RefreshCwIcon, ScanSearchIcon } from 'lucide-react'
import { toast } from 'sonner'

import { describeApiError } from '@/api/client'
import {
  useFailedRescrapeCount,
  useLibraryPending,
  useRescrapeFailedMovies,
  useStartLibraryScan
} from '@/api/library'
import { isTaskActive } from '@/api/tasks'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { LibraryScanButton } from '@/features/library/scan-button'
import { useTaskConnection } from '@/features/tasks/task-events'
import { notifyTaskError, useNotifyScanTask } from '@/features/tasks/task-toast'
import { useActiveScan } from '@/features/tasks/use-active-scan'
import { SettingRow, SettingsSection } from './shared'

export function MediaSyncSection() {
  const pending = useLibraryPending()
  const failed = useFailedRescrapeCount()
  const rescrape = useRescrapeFailedMovies()
  const startScan = useStartLibraryScan()
  const notifyScan = useNotifyScanTask()
  const { tasks, scans, active } = useActiveScan()
  const connection = useTaskConnection()
  const connected = connection.status === 'connected'
  const scanActive = active && !active.bulk_rescrape ? active : undefined
  const bulkActive = !!scans?.some(task => task.bulk_rescrape && isTaskActive(task))
  const busy = startScan.isPending || rescrape.isPending || !!scanActive || bulkActive
  const failedTotal = failed.data?.count ?? 0
  const failedDescription = failed.isPending
    ? '正在统计刮削失败的影片'
    : failed.isError
      ? '无法读取刮削失败的影片数量'
      : failedTotal
        ? `共 ${failedTotal} 部刮削失败，可批量提交`
        : '当前没有可重新刮削的失败影片'

  const submitScan = (rebuild: boolean) =>
    startScan.mutate(rebuild, {
      onSuccess: notifyScan,
      onError: error =>
        notifyTaskError(
          'scan:submit-error',
          rebuild ? '无法创建重建任务' : '无法创建扫描任务',
          describeApiError(error)
        )
    })

  return (
    <SettingsSection
      icon={<ScanSearchIcon className="size-4" />}
      title="媒体库同步"
      description={pending.data ? `待刮削 ${pending.data.count} 部影片` : undefined}
    >
      <SettingRow title="扫描并修复媒体库" description="发现新增文件、修复导出并重试刮削失败的影片">
        <LibraryScanButton
          rebuild={false}
          loading={tasks.isPending}
          scanning={!!scanActive}
          active={!!scanActive && !scanActive.rebuild}
          connected={connected}
          pending={startScan.isPending}
          blocked={bulkActive || rescrape.isPending}
          onSubmit={submitScan}
        />
      </SettingRow>
      <SettingRow title="重新刮削失败影片" description={failedDescription}>
        <Button
          variant="outline"
          disabled={busy || failed.isPending || failedTotal === 0}
          onClick={() =>
            rescrape.mutate(undefined, {
              onSuccess: result => {
                const parts: string[] = []
                if (result.created) parts.push(`已提交 ${result.created} 部`)
                if (result.reused) parts.push(`${result.reused} 部已在队列中`)
                if (result.skipped) parts.push(`${result.skipped} 部无法匹配来源已跳过`)
                toast.success(parts.length ? parts.join('，') : '没有需要重新刮削的影片')
              },
              onError: error => toast.error(describeApiError(error))
            })
          }
        >
          {rescrape.isPending || bulkActive ? (
            <Spinner data-icon="inline-start" />
          ) : (
            <RefreshCwIcon data-icon="inline-start" />
          )}
          {bulkActive ? '正在重新刮削' : '重新刮削失败影片'}
        </Button>
      </SettingRow>
      <SettingRow title="重建媒体库" description="重新刮削全部影片和重建索引">
        <LibraryScanButton
          rebuild
          loading={tasks.isPending}
          scanning={!!scanActive}
          active={!!scanActive && !!scanActive.rebuild}
          connected={connected}
          pending={startScan.isPending}
          blocked={bulkActive || rescrape.isPending}
          onSubmit={submitScan}
        />
      </SettingRow>
    </SettingsSection>
  )
}
