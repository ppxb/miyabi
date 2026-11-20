import { useRecordMovieView } from '@/api/browse-history'
import { useDiscoverMagnets, useDiscoverMovie } from '@/api/discover'
import { useLibraryMovie } from '@/api/library'
import { ErrorState, InlineError } from '@/components/error-state'
import type { MovieDetailTarget } from './dialog-context'
import { MovieHero } from './hero'
import { MovieMagnets } from './magnets'
import { MoviePreviews } from './previews'
import { MovieRecommendations } from './recommendations'
import { MovieDetailSkeleton } from './skeleton'

export function MovieDetailContent({ movie }: { movie: MovieDetailTarget }) {
  if ('libraryId' in movie) return <LibraryMovieDetail movieId={movie.libraryId} />
  return 'id' in movie ? (
    <IdentifiedMovieDetail movieId={movie.id} code={movie.code} />
  ) : (
    <IdentifiedMovieDetail movieId={movie.code} code={movie.code} />
  )
}

function LibraryMovieDetail({ movieId }: { movieId: number }) {
  const detail = useLibraryMovie(movieId)
  if (detail.isPending) return <MovieDetailSkeleton />
  if (!detail.data) {
    return (
      <ErrorState
        message="媒体库详情加载失败"
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
      <MovieHero movie={detail.data} libraryStatus={detail.data.scrape_status} />
      <MoviePlot plot={detail.data.plot} />
      <MoviePreviews images={detail.data.preview_images} />
      {detail.data.id ? (
        <LibraryCatalogueExtras movieId={detail.data.id} code={detail.data.code} />
      ) : null}
    </div>
  )
}

function MoviePlot({ plot }: { plot?: string }) {
  if (!plot) return null
  return (
    <section className="space-y-2">
      <h2 className="text-sm font-semibold">简介</h2>
      <p className="text-sm leading-6 whitespace-pre-line text-muted-foreground">{plot}</p>
    </section>
  )
}

// Catalogue extras remain available for linked films, but cannot block or
// replace the saved library metadata when the catalogue is unavailable.
function LibraryCatalogueExtras({ movieId, code }: { movieId: string; code: string }) {
  const detail = useDiscoverMovie(movieId, code)
  const magnets = useDiscoverMagnets(movieId, detail.data?.code ?? code)
  useRecordMovieView(movieId)
  return (
    <>
      <MovieMagnets movieID={movieId} query={magnets} />
      {detail.data ? (
        <>
          <MovieRecommendations title="TA（们）还出演过" movies={detail.data.actor_movies} />
          <MovieRecommendations title="你可能也喜欢" movies={detail.data.related_movies} />
        </>
      ) : null}
    </>
  )
}

function IdentifiedMovieDetail({ movieId, code }: { movieId: string; code?: string }) {
  const detail = useDiscoverMovie(movieId, code)
  const magnets = useDiscoverMagnets(movieId, detail.data?.code ?? code)
  useRecordMovieView(movieId)

  if (!detail.data && detail.isPending) return <MovieDetailSkeleton />
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
      <MoviePlot plot={detail.data.plot} />
      <MoviePreviews images={detail.data.preview_images} />
      <MovieMagnets movieID={movieId} query={magnets} />
      <MovieRecommendations title="TA（们）还出演过" movies={detail.data.actor_movies} />
      <MovieRecommendations title="你可能也喜欢" movies={detail.data.related_movies} />
    </div>
  )
}
