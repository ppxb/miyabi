import { useDiscoverMagnets, useDiscoverMovie } from '@/api/discover'
import { AppPage } from '@/components/app-page'
import { EmptyState } from '@/components/empty-state'
import { PageBackButton } from '@/components/page-back-button'
import { Button } from '@/components/ui/button'
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
            <p role="alert" className="text-sm text-destructive">
              刷新失败，请重试。
            </p>
          ) : null}
          <MovieHero movie={detail.data} />
          <MoviePreviews code={detail.data.code} images={detail.data.preview_images} />
          <MovieMagnets query={magnets} />
          <MovieRecommendations
            id="actor-movies-title"
            title="TA（们）还出演过"
            movies={detail.data.actor_movies}
          />
          <MovieRecommendations
            id="related-movies-title"
            title="你可能也喜欢"
            movies={detail.data.related_movies}
          />
        </div>
      ) : (
        <EmptyState
          emoji="Ò︵Ó"
          title="影片详情加载失败"
          actions={
            <Button type="button" variant="outline" size="sm" onClick={() => detail.refetch()}>
              重试
            </Button>
          }
        />
      )}
    </AppPage>
  )
}
