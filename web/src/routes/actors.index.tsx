import { createFileRoute } from '@tanstack/react-router'

import { ActorsCatalogPage } from '@/features/actors/catalog-page'
import { validateActorsSearch } from '@/features/actors/search'

export const Route = createFileRoute('/actors/')({
  validateSearch: validateActorsSearch,
  component: ActorsRoute
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
