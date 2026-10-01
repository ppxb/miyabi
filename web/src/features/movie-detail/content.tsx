import { useRecordMovieView } from '@/api/browse-history'
import { useDiscoverMagnets, useDiscoverMovie, useResolveDiscoverMovie } from '@/api/discover'
import { ErrorState, InlineError } from '@/components/error-state'
import type { MovieDetailTarget } from './dialog-context'
import { MovieHero } from './hero'
import { MovieMagnets } from './magnets'
import { MoviePreviews } from './previews'
import { MovieRecommendations } from './recommendations'
import { MovieDetailSkeleton } from './skeleton'

export function MovieDetailContent({ movie }: { movie: MovieDetailTarget }) {
  return 'id' in movie ? (
    <IdentifiedMovieDetail movieId={movie.id} />
  ) : (
    <MovieDetailByCode code={movie.code} />
  )
}

function MovieDetailByCode({ code }: { code: string }) {
  const identity = useResolveDiscoverMovie(code)
  if (identity.isPending) return <MovieDetailSkeleton />
  if (!identity.data) {
    return (
      <ErrorState
        message={`未能从 JavDB 确认 ${code} 的影片详情`}
        onRetry={() => void identity.refetch()}
        retrying={identity.isFetching}
      />
    )
  }
  return <IdentifiedMovieDetail key={identity.data.id} movieId={identity.data.id} />
}

function IdentifiedMovieDetail({ movieId }: { movieId: string }) {
  const detail = useDiscoverMovie(movieId)
  const magnets = useDiscoverMagnets(movieId)
  useRecordMovieView(movieId)

  if (detail.isPending) return <MovieDetailSkeleton />
  if (!detail.data) {
    return (
      <ErrorState
        message="影片详情加载失败"
        onRetry={() => void detail.refetch()}
        retrying={detail.isFetching}
      />
    )
  }

  return (
    <div className="space-y-10">
      {detail.isRefetchError ? (
        <InlineError onRetry={() => void detail.refetch()} retrying={detail.isFetching}>
          刷新失败，请重试。
        </InlineError>
      ) : null}
      <MovieHero movie={detail.data} />
      <MoviePreviews images={detail.data.preview_images} />
      <MovieMagnets movieID={movieId} query={magnets} />
      <MovieRecommendations title="TA（们）还出演过" movies={detail.data.actor_movies} />
      <MovieRecommendations title="你可能也喜欢" movies={detail.data.related_movies} />
    </div>
  )
}
