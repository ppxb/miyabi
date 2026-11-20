import { useEffect, useRef } from 'react'

import type { DiscoverMovie, MovieReference } from '@/api/discover'
import { useRecommendationMovie } from '@/api/discover'
import { MovieCardSkeleton, MovieGridLayout } from '@/components/movie'
import { Button } from '@/components/ui/button'
import { DiscoverMovieCard } from '@/features/discover/movie-card'
import { observeRecommendation } from './recommendation-visibility'

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
      <h2 className="text-xl font-semibold tracking-normal">{title}</h2>
      <MovieGridLayout>
        {visible.map(movie => (
          <RecommendationCard key={movie.id} movie={movie} />
        ))}
      </MovieGridLayout>
    </section>
  )
}

// A sparse recommendation only knows its code and thumbnail; render the same
// explore card with the known identity until its detail arrives.
function referenceMovie(movie: MovieReference): DiscoverMovie {
  return {
    id: movie.id,
    code: movie.code,
    title: '',
    origin_title: '',
    release_date: '',
    duration: 0,
    rating: 0,
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

function RecommendationCard({ movie }: { movie: MovieReference }) {
  const cardRef = useRef<HTMLDivElement>(null)
  const detail = useRecommendationMovie(movie.id)
  const { request, prioritize } = detail
  const data = detail.movie
  const failed = detail.isError && !detail.isFetching

  useEffect(() => {
    if (cardRef.current) return observeRecommendation(cardRef.current, request)
  }, [request])

  return (
    <div ref={cardRef} className="relative h-full min-w-0">
      {data || failed ? (
        <DiscoverMovieCard movie={data ?? referenceMovie(movie)} allowSubscribe={false} />
      ) : (
        <MovieCardSkeleton />
      )}
      {!data && failed ? (
        <Button
          type="button"
          variant="ghost"
          size="xs"
          className="absolute right-3 bottom-2"
          onClick={prioritize}
        >
          重试
        </Button>
      ) : null}
    </div>
  )
}
