import { createFileRoute } from '@tanstack/react-router'

import { AppPage } from '@/components/app-page'
import { PageHeader } from '@/components/page-header'
import { Card, CardContent } from '@/components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { AccountSection } from '@/features/settings/account-section'
import { AppearanceSection } from '@/features/settings/appearance-section'
import { DataSection } from '@/features/settings/data-section'
import { EmbySection } from '@/features/settings/emby-section'
import { LocalMediaSection } from '@/features/settings/local-media-section'
import { NetworkSection } from '@/features/settings/network-section'
import { PanSection } from '@/features/settings/pan-section'
import { PrivacySection } from '@/features/settings/privacy-section'
import { ScrapingSection } from '@/features/settings/scraping-section'
import { SubscriptionSection } from '@/features/settings/subscription-section'
import { TasksSection } from '@/features/settings/tasks-section'

export const Route = createFileRoute('/settings')({
  component: SettingsPage
})

const TABS = [
  { value: 'general', label: '通用' },
  { value: 'network', label: '网络代理' },
  { value: 'scraping', label: '刮削' },
  { value: 'subscription', label: '订阅及下载' },
  { value: 'sources', label: '影片来源' },
  { value: 'emby', label: 'Emby' },
  { value: 'tasks', label: '任务' },
  { value: 'data', label: '数据' }
]

function SettingsPage() {
  return (
    <AppPage showBackTop={false}>
      <PageHeader title="设置" description="管理应用选项和其他配置" />

      <Card>
        <CardContent className="space-y-6">
          <Tabs defaultValue="general" className="space-y-6">
            <TabsList className="justify-start overflow-x-auto overflow-y-hidden">
              {TABS.map(tab => (
                <TabsTrigger key={tab.value} value={tab.value} className="flex-none px-3">
                  {tab.label}
                </TabsTrigger>
              ))}
            </TabsList>

            <TabsContent value="general">
              <div className="space-y-8">
                <AppearanceSection />
                <AccountSection />
                <PrivacySection />
              </div>
            </TabsContent>

            <TabsContent value="network">
              <NetworkSection />
            </TabsContent>

            <TabsContent value="scraping">
              <ScrapingSection />
            </TabsContent>

            <TabsContent value="subscription">
              <SubscriptionSection />
            </TabsContent>

            <TabsContent value="sources">
              <div className="space-y-8">
                <LocalMediaSection />
                <PanSection />
              </div>
            </TabsContent>

            <TabsContent value="emby">
              <EmbySection />
            </TabsContent>

            <TabsContent value="tasks">
              <TasksSection />
            </TabsContent>

            <TabsContent value="data">
              <DataSection />
            </TabsContent>
          </Tabs>
        </CardContent>
      </Card>
    </AppPage>
  )
}
