import { useCanGoBack, useRouter } from '@tanstack/react-router'
import { useState } from 'react'
import {
  ArrowLeftIcon,
  CakeIcon,
  CalendarDaysIcon,
  CupSodaIcon,
  DropletIcon,
  ExternalLinkIcon,
  MapPinIcon,
  PersonStandingIcon,
  RulerIcon,
  type LucideIcon
} from 'lucide-react'

import { useActorDetail } from '@/api/actors'
import { imageURL } from '@/api/client'
import { AppPage } from '@/components/app-page'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { MediaImage } from '@/components/media-image'
import { MovieGridLayout } from '@/components/movie/movie-grid-layout'
import { MovieGridSkeleton } from '@/components/movie/movie-grid-skeleton'
import { StatList } from '@/components/stat-list'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { LibraryMovieCard } from '@/features/library/movie-card'
import { LazyLibraryPlayerDialog } from '@/features/library/player-dialog-lazy'
import { FollowButton } from './follow-button'

function ActorStats({ actor }: { actor: ReturnType<typeof useActorDetail>['data'] }) {
  if (!actor) return null
  // Show the whole strip for actors with any data; missing fields use "-".
  const text = (value?: string) => (value && value !== '-' ? value : '-')
  const stats: Array<{ label: string; value: string; icon: LucideIcon }> = [
    { label: '生日', value: text(actor.birthday), icon: CakeIcon },
    { label: '身高', value: actor.height ? `${actor.height} cm` : '-', icon: RulerIcon },
    { label: '三围', value: text(actor.measurements), icon: PersonStandingIcon },
    { label: '罩杯', value: text(actor.cup_size), icon: CupSodaIcon },
    { label: '血型', value: text(actor.blood_type), icon: DropletIcon },
    { label: '出身', value: text(actor.nationality), icon: MapPinIcon },
    { label: '出道', value: text(actor.debut_date), icon: CalendarDaysIcon }
  ]
  if (stats.every(stat => stat.value === '-')) return null

  return <StatList stats={stats} />
}

export function ActorDetailPage({ actorKey }: { actorKey: string }) {
  const detail = useActorDetail(actorKey)
  const [playingId, setPlayingId] = useState<number | null>(null)
  const router = useRouter()
  const canGoBack = useCanGoBack()
  if (detail.isPending) {
    return (
      <AppPage>
        <Skeleton className="h-4 w-24" />
        <div className="flex flex-col gap-4 sm:flex-row sm:items-center">
          <Skeleton className="size-32 rounded-full" />
          <div className="flex flex-col gap-2">
            <Skeleton className="h-10 w-56" />
            <Skeleton className="h-4 w-32" />
            <Skeleton className="h-4 w-40" />
          </div>
        </div>
        <div className="flex flex-wrap gap-2">
          {Array.from({ length: 5 }).map((_, index) => (
            <Skeleton key={index} className="h-20 min-w-24 flex-1 rounded-2xl" />
          ))}
        </div>
        <div className="grid grid-cols-3 gap-2 sm:grid-cols-4 md:grid-cols-6">
          {Array.from({ length: 6 }).map((_, index) => (
            <Skeleton key={index} className="aspect-3/4 w-full rounded-lg" />
          ))}
        </div>
        <Skeleton className="h-6 w-24" />
        <MovieGridSkeleton count={6} />
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
  const sourceLinks = actor.source_links?.length
    ? actor.source_links
    : actor.source_url
      ? [{ provider: '', url: actor.source_url }]
      : []

  return (
    <AppPage>
      <Button
        type="button"
        variant="ghost"
        // History back keeps the catalog's search and page in the URL.
        className="h-11 gap-2 self-start px-3 text-base"
        onClick={() => {
          if (canGoBack) router.history.back()
          else void router.navigate({ to: '/actors', replace: true })
        }}
      >
        <ArrowLeftIcon className="size-5" />
        返回
      </Button>

      <div className="flex flex-col gap-4 sm:flex-row sm:items-center">
        <Avatar className="size-32">
          <AvatarImage
            src={actor.avatar ? imageURL(actor.avatar) : undefined}
            alt={actor.name}
            loading="eager"
          />
          <AvatarFallback>{actor.name.slice(0, 1)}</AvatarFallback>
        </Avatar>
        <div className="flex flex-col gap-2">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-4xl font-bold">{actor.name}</h1>
            {actor.name_zht ? (
              <span className="text-lg text-muted-foreground">{actor.name_zht}</span>
            ) : null}
            {actor.enriching ? <Badge variant="secondary">后台补全中</Badge> : null}
            <FollowButton actorKey={actor.key} />
          </div>
          {actor.aliases?.length ? (
            <p className="text-sm text-muted-foreground">{actor.aliases.join(' / ')}</p>
          ) : null}
          <div className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
            <span>{actor.movie_count} 部作品</span>
            {actor.sources.map(source => (
              <Badge key={`${source.provider}:${source.source_id}`}>{source.provider}</Badge>
            ))}
            {sourceLinks.length ? (
              <>
                {sourceLinks.map(link => (
                  <Button
                    key={`${link.provider}:${link.url}`}
                    variant="link"
                    size="sm"
                    nativeButton={false}
                    render={<a href={link.url} target="_blank" rel="noreferrer" />}
                  >
                    {link.provider || '资料来源'}
                    <ExternalLinkIcon data-icon="inline-end" />
                  </Button>
                ))}
              </>
            ) : null}
          </div>
        </div>
      </div>

      <ActorStats actor={actor} />

      {actor.hobby || actor.skill ? (
        <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
          {actor.hobby ? (
            <>
              <dt className="text-muted-foreground">爱好</dt>
              <dd>{actor.hobby}</dd>
            </>
          ) : null}
          {actor.skill ? (
            <>
              <dt className="text-muted-foreground">特长</dt>
              <dd>{actor.skill}</dd>
            </>
          ) : null}
        </dl>
      ) : null}

      {actor.summary ? (
        <p className="text-sm leading-6 whitespace-pre-line text-muted-foreground">
          {actor.summary}
        </p>
      ) : null}

      {actor.image_count > 0 ? (
        <div className="flex flex-col gap-2">
          <h2 className="text-xl font-semibold">图集</h2>
          <div className="grid grid-cols-3 gap-2 sm:grid-cols-4 md:grid-cols-6">
            {Array.from({ length: Math.min(actor.image_count, 12) }).map((_, index) => (
              <div key={index} className="aspect-3/4 w-full overflow-hidden rounded-lg">
                <MediaImage
                  source={`/api/actors/${actor.key}/image/${index}`}
                  alt={`${actor.name} ${index + 1}`}
                  className="object-cover"
                />
              </div>
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
            <LibraryMovieCard key={movie.id} movie={movie} onPlay={() => setPlayingId(movie.id)} />
          ))}
        </MovieGridLayout>
      )}

      <LazyLibraryPlayerDialog movieId={playingId} onClose={() => setPlayingId(null)} />
    </AppPage>
  )
}
