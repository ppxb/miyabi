import { RefreshCwIcon, ScanSearchIcon } from 'lucide-react'
import { toast } from 'sonner'

import { describeApiError } from '@/api/client'
import { useLibraryPending, useSyncLibrary } from '@/api/library'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { LibraryScanButton } from '@/features/library/scan-button'
import { useTaskConnection } from '@/features/tasks/task-events'
import { useActiveScan } from '@/features/tasks/use-active-scan'
import { SettingRow, SettingsSection } from './shared'

export function MediaSyncSection() {
  const sync = useSyncLibrary()
  const pending = useLibraryPending()
  const { tasks, active } = useActiveScan()
  const connection = useTaskConnection()

  return (
    <SettingsSection icon={<ScanSearchIcon className="size-4" />} title="媒体库同步">
      <SettingRow
        title="同步新增文件"
        description={
          pending.data
            ? `发现新增文件并补全未处理影片（待补全 ${pending.data.count}）`
            : '发现新增文件并补全未处理影片的资料与预览'
        }
      >
        <Button
          disabled={sync.isPending}
          onClick={() =>
            sync.mutate(undefined, {
              onSuccess: result => toast.success(`已开始同步（${result.started} 个来源）`),
              onError: error => toast.error(describeApiError(error))
            })
          }
        >
          {sync.isPending ? (
            <Spinner data-icon="inline-start" />
          ) : (
            <RefreshCwIcon data-icon="inline-start" />
          )}
          同步新增文件
        </Button>
      </SettingRow>
      <SettingRow title="全库维护" description="全库扫描重建索引，或重新刮削全部影片">
        <LibraryScanButton
          loading={tasks.isPending}
          scanning={!!active}
          rebuilding={!!active?.rebuild}
          connected={connection.status === 'connected'}
        />
      </SettingRow>
    </SettingsSection>
  )
}
