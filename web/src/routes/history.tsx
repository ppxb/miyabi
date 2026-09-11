import { createFileRoute } from '@tanstack/react-router'

import { AppPage } from '@/components/app-page'
import { ErrorState } from '@/components/error-state'
import { WatchHistoryPage } from '@/features/history/page'

export const Route = createFileRoute('/history')({
  validateSearch: (search: Record<string, unknown>): { page?: number } => {
    const page = Number(search.page ?? 1)
    if (!Number.isInteger(page) || page < 1 || page > 100_000_000) {
      throw new Error('观看历史页码无效')
    }
    return page > 1 ? { page } : {}
  },
  component: HistoryRoute,
  errorComponent: () => (
    <AppPage>
      <ErrorState message="观看历史页码无效" />
    </AppPage>
  )
})

function HistoryRoute() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <WatchHistoryPage
      page={search.page ?? 1}
      onPageChange={page => void navigate({ search: page > 1 ? { page } : {}, resetScroll: false })}
    />
  )
}
