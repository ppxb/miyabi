import { createFileRoute } from '@tanstack/react-router'

import { AppPage } from '@/components/app-page'
import { PageHeader } from '@/components/page-header'
import { Card, CardContent } from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
import { AppearanceSection } from '@/features/settings/appearance-section'
import { DataSection } from '@/features/settings/data-section'
import { JavDBSection } from '@/features/settings/javdb-section'
import { PanSection } from '@/features/settings/pan-section'
import { PrivacySection } from '@/features/settings/privacy-section'
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
          <DataSection />
        </CardContent>
      </Card>
    </AppPage>
  )
}
