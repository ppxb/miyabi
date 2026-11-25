import { SUBSCRIPTION_PAGE_SIZE, useActorFeed, useSubscriptions } from '@/api/subscriptions'
import { AppPage } from '@/components/app-page'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { ListPagination } from '@/components/list-pagination'
import { MovieGridLayout } from '@/components/movie/movie-grid-layout'
import { MovieGridSkeleton } from '@/components/movie/movie-grid-skeleton'
import { PageHeader } from '@/components/page-header'
import { Skeleton } from '@/components/ui/skeleton'
import { DiscoverMovieCard } from '@/features/discover/movie-card'
import { ActorStrip } from './actor-strip'

import type { SubscriptionsSearch } from './search'

// The API caps a page at 100; every followed actor fits in one strip.
const ACTOR_LIMIT = 100

export function SubscriptionsPage({
  search,
  onSearchChange
}: {
  search: SubscriptionsSearch
  onSearchChange: (search: SubscriptionsSearch) => void
}) {
  const selectedID = search.actorID ?? null
  const page = search.page ?? 1
  const actors = useSubscriptions('actor', 1, ACTOR_LIMIT)
  const feed = useActorFeed(selectedID, page)

  return (
    <AppPage>
      <PageHeader title="关注" description="追踪演员新作" />
      {actors.isPending ? (
        <div className="flex gap-4 overflow-hidden pb-2">
          {Array.from({ length: 8 }, (_, index) => (
            <Skeleton key={index} className="size-16 shrink-0 rounded-full" />
          ))}
        </div>
      ) : actors.isError ? (
        <ErrorState
          message="关注的演员加载失败"
          onRetry={() => void actors.refetch()}
          retrying={actors.isFetching}
        />
      ) : actors.data.length === 0 ? (
        <EmptyState title="还没有关注演员，在「演员」详情页进行关注" />
      ) : (
        <>
          <ActorStrip
            actors={actors.data}
            selectedID={selectedID}
            onSelect={id => onSearchChange({ actorID: id ?? undefined })}
          />
          {feed.isPending ? (
            <MovieGridSkeleton />
          ) : feed.isError ? (
            <ErrorState
              message="演员新作加载失败"
              onRetry={() => void feed.refetch()}
              retrying={feed.isFetching}
            />
          ) : page === 1 && feed.data.length === 0 ? (
            <EmptyState
              title={selectedID === null ? '关注的演员暂时没有新作' : '这位演员暂时没有新作'}
            />
          ) : (
            <MovieGridLayout>
              {feed.data.map(item => (
                <DiscoverMovieCard
                  key={item.id}
                  movie={{
                    id: item.target_id,
                    code: item.code,
                    title: item.title,
                    cover: item.cover,
                    rating: 0,
                    release_date: item.release_date ?? ''
                  }}
                />
              ))}
            </MovieGridLayout>
          )}
          <ListPagination
            page={page}
            hasMore={(feed.data?.length ?? 0) === SUBSCRIPTION_PAGE_SIZE}
            disabled={feed.isFetching}
            onPageChange={next => onSearchChange({ ...search, page: next > 1 ? next : undefined })}
          />
        </>
      )}
    </AppPage>
  )
}
