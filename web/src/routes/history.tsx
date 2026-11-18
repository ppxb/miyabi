import { createFileRoute } from '@tanstack/react-router'

import { AppPage } from '@/components/app-page'
import { ErrorState } from '@/components/error-state'
import { HistoryPage } from '@/features/history/page'
import { parseSearchPage } from '@/lib/search-schema'

export const Route = createFileRoute('/history')({
  validateSearch: (search: Record<string, unknown>): { page?: number } => {
    const page = parseSearchPage(search.page)
    return page ? { page } : {}
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
    <HistoryPage
      page={search.page ?? 1}
      onPageChange={page => void navigate({ search: page > 1 ? { page } : {}, resetScroll: false })}
    />
  )
}
