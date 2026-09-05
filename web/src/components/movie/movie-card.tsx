import { Link } from '@tanstack/react-router'

import type { DiscoverMovie } from '@/api/discover'
import { MovieResourceBadges, MovieStateBadge } from '@/components/movie/movie-badges'
import { MovieCover } from '@/components/movie/movie-cover'
import { OverflowTooltip } from '@/components/overflow-tooltip'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'

export function MovieCard({ movie }: { movie: DiscoverMovie }) {
  return (
    <Link
      to="/discover/$movieId"
      params={{ movieId: movie.id }}
      aria-label={`查看 ${movie.code} 的详情`}
      className="block rounded-2xl outline-ring"
    >
      <Card
        size="sm"
        className="group h-full gap-0 overflow-hidden py-0 transition-shadow hover:shadow-xl"
      >
        <div className="relative aspect-3/2 overflow-hidden bg-muted">
          <MovieCover key={movie.cover} source={movie.cover} alt={movie.title} />

          <div className="absolute top-2 left-2 flex max-w-[calc(100%-1rem)] flex-wrap gap-1.5">
            <Badge variant="outline" className="bg-background/85 backdrop-blur">
              {movie.code}
            </Badge>
            <MovieStateBadge state={movie.state} />
          </div>
        </div>

        <CardContent className="space-y-2 p-3">
          <OverflowTooltip content={movie.title}>
            <h3 className="truncate text-sm leading-5 font-semibold">{movie.title}</h3>
          </OverflowTooltip>
          <p className="text-xs text-muted-foreground">{movie.release_date}</p>
          <div className="flex flex-wrap gap-1.5">
            <MovieResourceBadges movie={movie} />
          </div>
        </CardContent>
      </Card>
    </Link>
  )
}
