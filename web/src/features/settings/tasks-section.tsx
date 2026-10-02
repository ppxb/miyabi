import { ListChecksIcon } from 'lucide-react'

import { isScanTask, isTaskActive, useTasks } from '@/api/tasks'
import { InlineError } from '@/components/error-state'
import { SettingRow, SettingsSection } from '@/features/settings/shared'
import { LibraryPauseButton } from '@/features/tasks/library-pause-button'
import { ScanProgressView } from '@/features/tasks/scan-progress'
import { useTaskConnection } from '@/features/tasks/task-events'

export function TasksSection() {
  const tasks = useTasks()
  const connection = useTaskConnection()
  const scans = tasks.data?.filter(isScanTask)
  const activeScan = scans?.find(isTaskActive)

  return (
    <SettingsSection icon={<ListChecksIcon className="size-4" />} title="任务">
      <SettingRow
        title="媒体库进度"
        description="查看同步与重建进度，暂停会作用于所有扫描与刮削任务并保留进度"
      >
        {activeScan ? <LibraryPauseButton paused={!!activeScan.paused} /> : null}
      </SettingRow>
      {tasks.isPending ? <p className="text-xs text-muted-foreground">正在读取任务…</p> : null}
      {tasks.isError ? (
        <InlineError
          onRetry={connection.reconnect}
          retrying={connection.status === 'connecting'}
          retryLabel="重新连接"
        >
          无法读取任务，请启动后端服务后重试。
        </InlineError>
      ) : null}
      {scans?.length === 0 ? (
        <p className="text-xs text-muted-foreground">还没有媒体库任务。</p>
      ) : null}
      <div className="divide-y divide-border">
        {scans?.slice(0, 3).map(task => (
          <div key={task.id} className="space-y-2 py-3 first:pt-0 last:pb-0">
            {task.movie_id ? <p className="text-sm">{task.code} · 重新刮削</p> : null}
            <ScanProgressView task={task} />
            <p className="text-xs text-muted-foreground">
              {new Date(task.created_at).toLocaleString('zh-CN')}
            </p>
          </div>
        ))}
      </div>
    </SettingsSection>
  )
}
