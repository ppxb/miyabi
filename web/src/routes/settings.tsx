import { createFileRoute } from '@tanstack/react-router'
import { DatabaseIcon } from 'lucide-react'

import { AppPage } from '@/components/app-page'
import { PageHeader } from '@/components/page-header'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
import { AppearanceSection } from '@/features/settings/appearance-section'
import { JavDBSection } from '@/features/settings/javdb-section'
import { PanSection } from '@/features/settings/pan-section'
import { PrivacySection } from '@/features/settings/privacy-section'
import { SettingRow, SettingsSection } from '@/features/settings/shared'
import { TasksSection } from '@/features/settings/tasks-section'

export const Route = createFileRoute('/settings')({
  component: SettingsPage
})

function SettingsPage() {
  return (
    <AppPage showBackTop={false}>
      <PageHeader title="设置" description="管理 115、JavDB 和本地应用选项" />

      <Card>
        <CardContent className="space-y-8">
          <AppearanceSection />
          <Separator />
          <PrivacySection />
          <Separator />
          <JavDBSection />
          <Separator />
          <PanSection />
          <Separator />
          <TasksSection />
          <Separator />
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
