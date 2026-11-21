import type { MovieReference } from '@/api/discover'
import { MovieGridLayout } from '@/components/movie'
import { DiscoverMovieCard, type DiscoverMovieCardData } from '@/features/discover/movie-card'

export function MovieRecommendations({
  title,
  movies
}: {
  title: string
  movies: MovieReference[]
}) {
  const visible = (movies ?? []).slice(0, 8)
  if (visible.length === 0) return null

  return (
    <section className="space-y-4">
      <h2 className="text-xl font-semibold">{title}</h2>
      <MovieGridLayout>
        {visible.map(movie => (
          <DiscoverMovieCard key={movie.id} movie={referenceMovie(movie)} />
        ))}
      </MovieGridLayout>
    </section>
  )
}

// Recommendations arrive fully populated in the detail response, so the explore
// card renders straight away without per-card detail requests or skeletons.
function referenceMovie(movie: MovieReference): DiscoverMovieCardData {
  return {
    id: movie.id,
    code: movie.code,
    title: movie.title ?? '',
    release_date: movie.release_date ?? '',
    rating: movie.rating ?? 0,
    cover: movie.cover || movie.thumbnail
  }
}
