import { createFileRoute } from '@tanstack/react-router'
import { CloudIcon, DatabaseIcon, ListChecksIcon } from 'lucide-react'

import { AppPage } from '@/components/app-page'
import { PageHeader } from '@/components/page-header'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import { JavDBSection } from '@/features/settings/javdb-section'
import { PrivacySection } from '@/features/settings/privacy-section'
import { SettingRow, SettingsSection } from '@/features/settings/shared'

export const Route = createFileRoute('/settings')({
  component: SettingsPage
})

function SettingsPage() {
  return (
    <AppPage contentClassName="max-w-5xl gap-8" showBackTop={false}>
      <PageHeader title="设置" description="管理 115、JavDB 和本地应用选项" />

      <Card>
        <CardContent className="space-y-8">
          <PrivacySection />
          <hr className="border-border" />
          <JavDBSection />
          <hr className="border-border" />
          <SettingsSection icon={<CloudIcon className="size-4" />} title="115 网盘">
            <SettingRow title="账号与媒体目录" description="登录账号、选择媒体目录和管理扫描选项">
              <Badge variant="secondary">即将接入</Badge>
            </SettingRow>
          </SettingsSection>
          <hr className="border-border" />
          <SettingsSection icon={<ListChecksIcon className="size-4" />} title="任务">
            <SettingRow title="任务进度" description="查看扫描、刮削和下载任务进度">
              <Badge variant="secondary">即将接入</Badge>
            </SettingRow>
          </SettingsSection>
          <hr className="border-border" />
          <SettingsSection icon={<DatabaseIcon className="size-4" />} title="数据与缓存">
            <SettingRow title="数据目录" description="管理 SQLite 索引和图片缓存位置">
              <Badge variant="secondary">即将接入</Badge>
            </SettingRow>
          </SettingsSection>
        </CardContent>
      </Card>
    </AppPage>
  )
}
