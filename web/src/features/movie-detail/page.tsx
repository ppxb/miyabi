import { useDiscoverMagnets, useDiscoverMovie } from '@/api/discover'
import { AppPage } from '@/components/app-page'
import { ErrorState, InlineError } from '@/components/error-state'
import { PageBackButton } from '@/components/page-back-button'
import { MovieHero } from './hero'
import { MovieMagnets } from './magnets'
import { MoviePreviews } from './previews'
import { MovieRecommendations } from './recommendations'
import { MovieDetailSkeleton } from './skeleton'

export function MovieDetailPage({ movieId }: { movieId: string }) {
  const detail = useDiscoverMovie(movieId)
  const magnets = useDiscoverMagnets(movieId)

  return (
    <AppPage className="sm:px-6 lg:px-8" contentClassName="max-w-7xl gap-8">
      <PageBackButton />

      {detail.isPending ? (
        <MovieDetailSkeleton />
      ) : detail.data ? (
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
      ) : (
        <ErrorState
          message="影片详情加载失败"
          onRetry={() => void detail.refetch()}
          retrying={detail.isFetching}
        />
      )}
    </AppPage>
  )
}
