import { createFileRoute } from '@tanstack/react-router'

import { AppPage } from '@/components/app-page'
import { EmptyState } from '@/components/empty-state'
import { PageHeader } from '@/components/page-header'

export const Route = createFileRoute('/')({
  component: LibraryPage
})

function LibraryPage() {
  return (
    <AppPage>
      <PageHeader title="媒体库" description="来自 115 网盘的影片索引" />
      <EmptyState className="min-h-0 flex-1" emoji="(˙ᯅ˙)" title="媒体库还是空的" />
    </AppPage>
  )
}
