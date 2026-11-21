import { createFileRoute } from '@tanstack/react-router'

import { AppPage } from '@/components/app-page'
import { ErrorState } from '@/components/error-state'
import { ActorsCatalogPage } from '@/features/actors/catalog-page'
import { parseSearchPage } from '@/lib/search-schema'

export const Route = createFileRoute('/actors/')({
  validateSearch: (search: Record<string, unknown>): { q?: string; page?: number } => {
    const q = typeof search.q === 'string' && search.q.trim() !== '' ? search.q : undefined
    return { q, page: parseSearchPage(search.page) }
  },
  component: ActorsRoute,
  errorComponent: () => (
    <AppPage>
      <ErrorState message="演员筛选条件无效" />
    </AppPage>
  )
})

function ActorsRoute() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <ActorsCatalogPage
      q={search.q}
      page={search.page ?? 1}
      onSearchChange={next => void navigate({ search: next, resetScroll: false })}
    />
  )
}
