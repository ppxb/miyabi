import type { DiscoverMovie } from '@/api/discover'
import { imageURL } from '@/api/client'
import { OverflowTooltip } from '@/components/overflow-tooltip'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'

export function MovieCard({ movie }: { movie: DiscoverMovie }) {
  return (
    <Card size="sm" className="group gap-0 overflow-hidden py-0 transition-shadow hover:shadow-xl">
      <div className="relative aspect-3/2 overflow-hidden bg-muted">
        <img
          src={imageURL(movie.cover)}
          alt={movie.title}
          loading="lazy"
          decoding="async"
          className="h-full w-full object-contain"
        />

        <div className="absolute top-2 left-2 flex max-w-[calc(100%-1rem)] flex-wrap gap-1.5">
          <Badge className="border-white/15 bg-black/55 text-white backdrop-blur">
            {movie.code}
          </Badge>
          <MovieStateBadge movie={movie} />
        </div>
      </div>

      <CardContent className="space-y-2 p-3">
        <OverflowTooltip content={movie.title}>
          <h3 className="truncate text-sm leading-5 font-semibold" tabIndex={0}>
            {movie.title}
          </h3>
        </OverflowTooltip>
        <div className="flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
          <span>{movie.release_date}</span>
          {movie.rating > 0 ? <span className="ml-auto">{movie.rating.toFixed(1)}</span> : null}
        </div>
        <div className="flex flex-wrap gap-1.5">
          {movie.has_subtitle ? <Badge variant="secondary">字幕</Badge> : null}
          {movie.has_preview ? <Badge variant="secondary">预览</Badge> : null}
          {movie.magnets_count > 0 ? <Badge variant="outline">含磁力</Badge> : null}
          {movie.release_status === 'upcoming' ? <Badge variant="outline">即将发行</Badge> : null}
        </div>
      </CardContent>
    </Card>
  )
}

function MovieStateBadge({ movie }: { movie: DiscoverMovie }) {
  if (movie.state === 'in_library') {
    return <Badge className="bg-emerald-600 text-white">已入库</Badge>
  }
  if (movie.state === 'saving') {
    return <Badge className="bg-amber-500 text-white">保存中</Badge>
  }
  return null
}
