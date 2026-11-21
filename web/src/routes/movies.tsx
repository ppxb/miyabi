import { createFileRoute } from '@tanstack/react-router'

import { MoviesPage } from '@/features/movies/page'
import { validateMoviesSearch } from '@/features/movies/search'

export const Route = createFileRoute('/movies')({
  validateSearch: validateMoviesSearch,
  component: MoviesRoute
})

function MoviesRoute() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <MoviesPage
      search={search}
      onSearchChange={next => void navigate({ search: next, resetScroll: false })}
    />
  )
}
