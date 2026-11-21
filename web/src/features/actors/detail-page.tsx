import { Link } from '@tanstack/react-router'
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

function ActorStats({ actor }: { actor: ReturnType<typeof useActorDetail>['data'] }) {
  if (!actor) return null
  const stats: Array<{ label: string; value: string; icon: LucideIcon }> = []
  if (actor.birthday) stats.push({ label: '生日', value: actor.birthday, icon: CakeIcon })
  if (actor.height) stats.push({ label: '身高', value: `${actor.height} cm`, icon: RulerIcon })
  if (actor.measurements) {
    stats.push({ label: '三围', value: actor.measurements, icon: PersonStandingIcon })
  }
  if (actor.cup_size) stats.push({ label: '罩杯', value: actor.cup_size, icon: CupSodaIcon })
  if (actor.blood_type) stats.push({ label: '血型', value: actor.blood_type, icon: DropletIcon })
  if (actor.nationality) stats.push({ label: '出身', value: actor.nationality, icon: MapPinIcon })
  if (actor.debut_date) {
    stats.push({ label: '出道', value: actor.debut_date, icon: CalendarDaysIcon })
  }
  if (stats.length === 0) return null

  return <StatList stats={stats} />
}

export function ActorDetailPage({ actorKey }: { actorKey: string }) {
  const detail = useActorDetail(actorKey)
  const [playingId, setPlayingId] = useState<number | null>(null)
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

  return (
    <AppPage>
      <Link
        to="/actors"
        className="inline-flex w-fit items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeftIcon className="size-4" />
        返回
      </Link>

      <div className="flex flex-col gap-4 sm:flex-row sm:items-center">
        <Avatar className="size-32">
          <AvatarImage src={actor.avatar ? imageURL(actor.avatar) : undefined} alt={actor.name} loading="eager" />
          <AvatarFallback>{actor.name.slice(0, 1)}</AvatarFallback>
        </Avatar>
        <div className="flex flex-col gap-2">
          <div className="flex flex-wrap items-baseline gap-2">
            <h1 className="text-4xl font-bold">{actor.name}</h1>
            {actor.name_zht ? (
              <span className="text-lg text-muted-foreground">{actor.name_zht}</span>
            ) : null}
          </div>
          {actor.aliases?.length ? (
            <p className="text-sm text-muted-foreground">{actor.aliases.join(' / ')}</p>
          ) : null}
          <div className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
            <span>{actor.movie_count} 部作品</span>
            {actor.sources.map(source => (
              <Badge key={`${source.provider}:${source.source_id}`}>{source.provider}</Badge>
            ))}
            {actor.source_url ? (
              <Button
                variant="link"
                size="sm"
                nativeButton={false}
                render={<a href={actor.source_url} target="_blank" rel="noreferrer" />}
              >
                资料来源
                <ExternalLinkIcon data-icon="inline-end" />
              </Button>
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
