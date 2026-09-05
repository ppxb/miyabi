import { createFileRoute } from '@tanstack/react-router'

import { MovieDetailPage } from '@/features/movie-detail/page'

export const Route = createFileRoute('/discover_/$movieId')({
  component: MovieDetailRoute
})

function MovieDetailRoute() {
  const { movieId } = Route.useParams()
  return <MovieDetailPage key={movieId} movieId={movieId} />
}
