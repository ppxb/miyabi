import { CalendarIcon, ClockIcon, StarIcon, type LucideIcon } from 'lucide-react'

import type { DiscoverMovie, DiscoverMovieDetail } from '@/api/discover'
import type { LibraryMovieDetail } from '@/api/library'
import { MovieStateBadge } from '@/components/movie/movie-badges'
import { MovieCover } from '@/components/movie/movie-cover'
import { StatList } from '@/components/stat-list'
import { Badge } from '@/components/ui/badge'
import { MovieMetadata } from './metadata'

export function MovieHero({
  movie,
  libraryStatus
}: {
  movie: DiscoverMovieDetail | LibraryMovieDetail
  libraryStatus?: LibraryMovieDetail['scrape_status']
}) {
  return (
    <section className="grid items-start gap-6 lg:grid-cols-[minmax(0,1.1fr)_minmax(0,1fr)] lg:gap-8">
      <div className="aspect-3/2 overflow-hidden rounded-2xl bg-muted ring-1 ring-foreground/10">
        <MovieCover source={movie.cover} loading="eager" />
      </div>

      <div className="min-w-0 space-y-5 py-1">
        <div className="space-y-3">
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="outline" className="tabular-nums">
              {movie.code}
            </Badge>
            {libraryStatus && movie.has_subtitle ? <Badge variant="outline">中文</Badge> : null}
            {libraryStatus ? (
              <Badge variant={libraryStatus === 'done' ? 'library' : 'secondary'}>
                {{ done: '已入库', pending: '待刮削', failed: '刮削失败' }[libraryStatus]}
              </Badge>
            ) : (
              <MovieStateBadge movie={movie} hideViewed />
            )}
          </div>
          <h1 className="text-xl leading-snug font-bold tracking-normal sm:text-2xl">
            {movie.title || movie.code}
          </h1>
        </div>

        <MovieStats movie={movie} />

        <MovieMetadata movie={movie} />
      </div>
    </section>
  )
}

function MovieStats({
  movie
}: {
  movie: Pick<DiscoverMovie, 'release_date' | 'duration' | 'rating' | 'rating_max'>
}) {
  const stats: Array<{ label: string; value: string; icon: LucideIcon }> = []
  if (movie.release_date) {
    stats.push({ label: '发行日期', value: movie.release_date, icon: CalendarIcon })
  }
  if (movie.duration > 0) {
    stats.push({ label: '片长', value: `${movie.duration} 分钟`, icon: ClockIcon })
  }
  stats.push({
    label: '评分',
    value:
      movie.rating > 0
        ? movie.rating_max
          ? `${movie.rating.toFixed(1)} / ${movie.rating_max}`
          : movie.rating.toFixed(1)
        : '暂无评分',
    icon: StarIcon
  })

  return <StatList stats={stats} />
}
