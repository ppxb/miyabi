import { createFileRoute } from '@tanstack/react-router'

import { AppPage } from '@/components/app-page'
import { ErrorState } from '@/components/error-state'
import { MoviesPage } from '@/features/movies/page'
import { validateMoviesSearch } from '@/features/movies/search'

export const Route = createFileRoute('/movies')({
  validateSearch: validateMoviesSearch,
  component: MoviesRoute,
  errorComponent: () => (
    <AppPage>
      <ErrorState message="影片筛选条件无效" />
    </AppPage>
  )
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
