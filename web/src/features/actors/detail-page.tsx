import { useActorDetail } from '@/api/actors'
import { imageURL } from '@/api/client'
import { AppPage } from '@/components/app-page'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { MovieGridLayout } from '@/components/movie/movie-grid-layout'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { Badge } from '@/components/ui/badge'
import { LibraryMovieCard } from '@/features/library/movie-card'

function displayAvatar(src?: string) {
  if (!src) return undefined
  return src.startsWith('/api/actors/') ? src : imageURL(src)
}

export function ActorDetailPage({ actorKey }: { actorKey: string }) {
  const detail = useActorDetail(actorKey)
  if (detail.isPending) {
    return (
      <AppPage>
        <p className="text-sm text-muted-foreground">正在读取演员…</p>
      </AppPage>
    )
  }
  if (detail.isError || !detail.data) {
    return (
      <AppPage>
        <ErrorState
          message="无法读取演员，请检查后端服务后重试"
          onRetry={() => void detail.refetch()}
          retrying={detail.isFetching}
        />
      </AppPage>
    )
  }

  const actor = detail.data
  const films = actor.films?.movies ?? []

  return (
    <AppPage>
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center">
        <Avatar size="lg" className="size-24">
          <AvatarImage src={displayAvatar(actor.avatar)} alt={actor.name} />
          <AvatarFallback>{actor.name.slice(0, 1)}</AvatarFallback>
        </Avatar>
        <div className="flex flex-col gap-2">
          <div className="flex flex-wrap items-baseline gap-2">
            <h1 className="text-3xl font-bold">{actor.name}</h1>
            {actor.name_zht ? (
              <span className="text-muted-foreground">{actor.name_zht}</span>
            ) : null}
          </div>
          {actor.aliases.length > 0 ? (
            <p className="text-sm text-muted-foreground">别名：{actor.aliases.join(' / ')}</p>
          ) : null}
          <div className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
            <span>{actor.movie_count} 部作品</span>
            {actor.sources.map(source => (
              <Badge key={`${source.provider}:${source.source_id}`}>{source.provider}</Badge>
            ))}
          </div>
        </div>
      </div>

      {actor.image_count > 0 ? (
        <div className="flex flex-col gap-2">
          <h2 className="text-xl font-semibold">图片</h2>
          <div className="grid grid-cols-3 gap-2 sm:grid-cols-4 md:grid-cols-6">
            {Array.from({ length: Math.min(actor.image_count, 12) }).map((_, index) => (
              <img
                key={index}
                src={`/api/actors/${actor.key}/image/${index}`}
                alt={`${actor.name} ${index + 1}`}
                loading="lazy"
                className="aspect-[3/4] w-full rounded-lg object-cover"
              />
            ))}
          </div>
        </div>
      ) : null}

      <h2 className="text-xl font-semibold">参演作品</h2>
      {films.length === 0 ? (
        <EmptyState title="暂无参演作品" />
      ) : (
        <MovieGridLayout>
          {films.map(movie => (
            <LibraryMovieCard key={movie.id} movie={movie} playIcon={false} />
          ))}
        </MovieGridLayout>
      )}
    </AppPage>
  )
}