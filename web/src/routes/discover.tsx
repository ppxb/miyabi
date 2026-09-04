import { createFileRoute } from '@tanstack/react-router'

import { AppPage } from '@/components/app-page'
import { PageHeader } from '@/components/page-header'
import { DiscoverContent } from '@/features/discover/page'

export const Route = createFileRoute('/discover')({
  component: DiscoverPage
})

function DiscoverPage() {
  return (
    <AppPage>
      <PageHeader title="发现" description="浏览 JavDB 的最新发行、即将发行和分类内容" />
      <DiscoverContent />
    </AppPage>
  )
}
