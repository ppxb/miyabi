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
      <div className="flex flex-1 items-center justify-center rounded-3xl border border-dashed border-border/80 bg-card/30 px-6 py-20">
        <EmptyState
          className="min-h-0"
          emoji="🧭"
          title="发现功能即将上线"
          description="JavDB 接入后，这里会展示最新已发行、即将发行、分类筛选和搜索。"
        />
      </div>
    </AppPage>
  )
}
