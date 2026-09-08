import { CalendarIcon, ClockIcon, StarIcon, type LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'

import type { DiscoverMovie, DiscoverMovieDetail } from '@/api/discover'
import { MovieResourceBadges, MovieStateBadge } from '@/components/movie/movie-badges'
import { MovieCover } from '@/components/movie/movie-cover'
import { Badge } from '@/components/ui/badge'
import { MetadataLink } from './metadata-link'

export function MovieHero({ movie }: { movie: DiscoverMovieDetail }) {
  return (
    <section className="grid items-start gap-6 lg:grid-cols-[minmax(0,1.1fr)_minmax(0,1fr)] lg:gap-8">
      <div className="aspect-3/2 overflow-hidden rounded-2xl bg-muted ring-1 ring-foreground/10">
        <MovieCover key={movie.cover} source={movie.cover} loading="eager" />
      </div>

      <div className="min-w-0 space-y-5 py-1">
        <div className="space-y-3">
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="outline" className="tabular-nums">
              {movie.code}
            </Badge>
            <MovieStateBadge state={movie.state} />
          </div>
          <h1 className="text-2xl leading-tight font-bold tracking-normal sm:text-3xl">
            {movie.title}
          </h1>
          {movie.origin_title && movie.origin_title !== movie.title ? (
            <p className="text-sm leading-6 text-muted-foreground">{movie.origin_title}</p>
          ) : null}
        </div>

        <MovieStats movie={movie} />

        <div className="flex flex-wrap gap-2">
          <MovieResourceBadges movie={movie} />
        </div>

        <dl className="space-y-3 text-sm">
          {movie.maker ? (
            <EntityRow label="厂牌">
              <MetadataLink kind="maker" {...movie.maker} />
            </EntityRow>
          ) : null}
          {movie.series ? (
            <EntityRow label="系列">
              <MetadataLink kind="series" {...movie.series} />
            </EntityRow>
          ) : null}
          {movie.actors.length > 0 ? (
            <EntityRow label="演员">
              {movie.actors.map((actor, index) => (
                <span key={actor.id} className="inline-flex items-center gap-3">
                  {index > 0 ? <span className="text-muted-foreground">/</span> : null}
                  <MetadataLink kind="actor" id={actor.id} name={actor.name} />
                </span>
              ))}
            </EntityRow>
          ) : null}
          {movie.director ? (
            <EntityRow label="导演">
              <MetadataLink kind="director" {...movie.director} />
            </EntityRow>
          ) : null}
          {movie.tags.length > 0 ? (
            <div className="flex items-start gap-4">
              <dt className="w-10 shrink-0 pt-0.5 text-xs leading-5 text-muted-foreground">标签</dt>
              <dd className="flex min-w-0 flex-wrap gap-2">
                {movie.tags.map(tag => (
                  <MetadataLink
                    key={tag.id}
                    kind="tag"
                    id={tag.id}
                    name={tag.name}
                    zone={movie.zone}
                    badge
                  />
                ))}
              </dd>
            </div>
          ) : null}
        </dl>
      </div>
    </section>
  )
}

function MovieStats({ movie }: { movie: DiscoverMovie }) {
  const stats: Array<{ label: string; value: string; icon: LucideIcon }> = []
  if (movie.release_date) {
    stats.push({ label: '发行日期', value: movie.release_date, icon: CalendarIcon })
  }
  if (movie.duration > 0) {
    stats.push({ label: '片长', value: `${movie.duration} 分钟`, icon: ClockIcon })
  }
  if (movie.rating > 0) {
    stats.push({ label: 'JavDB 评分', value: movie.rating.toFixed(1), icon: StarIcon })
  }
  if (stats.length === 0) return null

  return (
    <dl className="flex flex-wrap divide-x divide-border/60 rounded-2xl bg-muted py-1 ring-1 ring-border dark:bg-card/60 dark:ring-0">
      {stats.map(stat => (
        <div
          key={stat.label}
          className="flex min-w-0 flex-1 flex-col items-center gap-2 p-3 sm:p-4"
        >
          <dt className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <stat.icon className="size-4" />
            {stat.label}
          </dt>
          <dd className="text-sm font-semibold tabular-nums sm:text-base">{stat.value}</dd>
        </div>
      ))}
    </dl>
  )
}

function EntityRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex items-start gap-4">
      <dt className="w-10 leading-6 text-muted-foreground">{label}</dt>
      <dd className="flex min-w-0 flex-wrap gap-x-3 gap-y-1 leading-6">{children}</dd>
    </div>
  )
}
