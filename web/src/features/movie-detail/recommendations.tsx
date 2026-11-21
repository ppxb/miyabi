import type { DiscoverMovie, MovieReference } from '@/api/discover'
import { MovieGridLayout } from '@/components/movie'
import { DiscoverMovieCard } from '@/features/discover/movie-card'

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
          <DiscoverMovieCard key={movie.id} movie={referenceMovie(movie)} allowSubscribe={false} />
        ))}
      </MovieGridLayout>
    </section>
  )
}

// Recommendations arrive fully populated in the detail response, so the explore
// card renders straight away without per-card detail requests or skeletons.
function referenceMovie(movie: MovieReference): DiscoverMovie {
  return {
    id: movie.id,
    code: movie.code,
    title: movie.title ?? '',
    origin_title: '',
    release_date: movie.release_date ?? '',
    duration: 0,
    rating: movie.rating ?? 0,
    thumbnail: movie.thumbnail,
    cover: movie.thumbnail,
    preview_images: [],
    preview_video: '',
    magnets_count: 0,
    has_subtitle: false,
    has_preview: false,
    actors: [],
    tags: [],
    release_status: 'released'
  }
}
