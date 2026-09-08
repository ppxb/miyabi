import { Link } from '@tanstack/react-router'
import { ListChecksIcon } from 'lucide-react'

import { useTasks } from '@/api/tasks'
import { Button } from '@/components/ui/button'
import { SettingRow, SettingsSection } from '@/features/settings/shared'
import { ScanProgressView } from '@/features/tasks/scan-progress'

export function TasksSection() {
  const tasks = useTasks()

  return (
    <SettingsSection icon={<ListChecksIcon className="size-4" />} title="任务">
      <SettingRow title="媒体库进度" description="查看扫描与刮削进度，离开页面后任务继续在后台执行">
        <Button asChild variant="outline" size="sm">
          <Link to="/">进入媒体库</Link>
        </Button>
      </SettingRow>
      {tasks.isPending ? <p className="text-xs text-muted-foreground">正在读取任务…</p> : null}
      {tasks.isError ? (
        <div className="flex items-center justify-between gap-3">
          <p className="text-xs text-muted-foreground">无法读取任务，请启动后端服务后重试。</p>
          <Button variant="outline" size="sm" onClick={() => void tasks.refetch()}>
            重试
          </Button>
        </div>
      ) : null}
      {tasks.data?.length === 0 ? (
        <p className="text-xs text-muted-foreground">还没有扫描任务。</p>
      ) : null}
      <div className="divide-y divide-border">
        {tasks.data?.slice(0, 3).map(task => (
          <div key={task.id} className="space-y-2 py-3 first:pt-0 last:pb-0">
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
