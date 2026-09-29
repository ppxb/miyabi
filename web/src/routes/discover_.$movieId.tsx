import { createFileRoute } from '@tanstack/react-router'

import { validateMainSearch } from '@/features/discover/search'
import { MovieDetailPage } from '@/features/movie-detail/page'

export const Route = createFileRoute('/discover_/$movieId')({
  validateSearch: validateMainSearch,
  component: MovieDetailRoute
})

function MovieDetailRoute() {
  const { movieId } = Route.useParams()
  return <MovieDetailPage key={movieId} movieId={movieId} />
}
