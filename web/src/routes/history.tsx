import { createFileRoute } from '@tanstack/react-router'

import { HistoryPage } from '@/features/history/page'
import { validateHistorySearch } from '@/features/history/search'

export const Route = createFileRoute('/history')({
  validateSearch: validateHistorySearch,
  component: HistoryRoute
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
