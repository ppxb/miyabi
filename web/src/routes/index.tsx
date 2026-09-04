import { createFileRoute } from '@tanstack/react-router'

import { useHealth } from '@/api/health'
import { AppPage } from '@/components/app-page'
import { EmptyState } from '@/components/empty-state'
import { PageHeader } from '@/components/page-header'

export const Route = createFileRoute('/')({
  component: LibraryPage
})

function LibraryPage() {
  const health = useHealth()

  return (
    <AppPage>
      <PageHeader title="媒体库" description="来自 115 网盘的影片索引">
        <div className="inline-flex items-center gap-2 rounded-full border border-border/70 bg-muted/40 px-3 py-1.5 text-xs text-muted-foreground">
          <span
            className={`size-2 rounded-full ${health.data?.status === 'ok' ? 'bg-emerald-400' : health.isError ? 'bg-red-400' : 'bg-amber-400'}`}
          />
          {health.data?.status === 'ok' ? '后端已连接' : health.isError ? '后端不可用' : '正在连接'}
        </div>
      </PageHeader>

      <div className="flex flex-1 items-center justify-center rounded-3xl border border-dashed border-border/80 bg-card/30 px-6 py-20">
        <EmptyState
          className="min-h-0"
          emoji="🍿"
          title="媒体库还是空的"
          description="接入 115 并扫描影片目录后，已入库内容会显示在这里。"
        />
      </div>
    </AppPage>
  )
}
