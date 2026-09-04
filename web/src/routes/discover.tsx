import { createFileRoute } from '@tanstack/react-router'

import { AppPage } from '@/components/app-page'
import { EmptyState } from '@/components/empty-state'
import { PageHeader } from '@/components/page-header'

export const Route = createFileRoute('/discover')({
  component: DiscoverPage
})

function DiscoverPage() {
  return (
    <AppPage>
      <PageHeader title="发现" description="浏览 JavDB 的最新发行、即将发行和分类内容" />
      <EmptyState className="min-h-0 flex-1" emoji="(･o･;)" title="发现功能即将上线" />
    </AppPage>
  )
}
