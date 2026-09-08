import { Link } from '@tanstack/react-router'
import { FilmIcon } from 'lucide-react'
import type { ReactNode } from 'react'

import type { DiscoverMovie } from '@/api/discover'
import { MovieResourceBadges, MovieStateBadge } from '@/components/movie/movie-badges'
import { MovieCover } from '@/components/movie/movie-cover'
import { OverflowTooltip } from '@/components/overflow-tooltip'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'

export function DiscoverMovieCard({ movie }: { movie: DiscoverMovie }) {
  return (
    <Link
      to="/discover/$movieId"
      params={{ movieId: movie.id }}
      className="block rounded-2xl outline-ring"
    >
      <MovieCard
        movie={movie}
        description={movie.release_date}
        state={<MovieStateBadge state={movie.state} />}
      >
        <MovieResourceBadges movie={movie} />
      </MovieCard>
    </Link>
  )
}

export function MovieCard({
  movie,
  description,
  state,
  children
}: {
  movie: { code: string; title: string; cover?: string }
  description: ReactNode
  state?: ReactNode
  children: ReactNode
}) {
  const title = movie.title || movie.code
  return (
    <Card size="sm" className="h-full gap-0 overflow-hidden py-0 transition-shadow hover:shadow-xl">
      <div className="relative flex aspect-3/2 items-center justify-center overflow-hidden bg-muted">
        {movie.cover ? (
          <div className="absolute inset-0">
            <MovieCover source={movie.cover} />
          </div>
        ) : (
          <FilmIcon className="size-8 text-muted-foreground/60" />
        )}
        <div className="absolute top-2 left-2 flex max-w-[calc(100%-1rem)] flex-wrap gap-1.5">
          <Badge variant="outline" className="max-w-full truncate bg-background/85 backdrop-blur">
            {movie.code}
          </Badge>
          {state}
        </div>
      </div>
      <CardContent className="min-w-0 space-y-2 p-3">
        <OverflowTooltip content={title}>
          <h3 className="truncate text-sm leading-5 font-semibold">{title}</h3>
        </OverflowTooltip>
        <p className="text-xs text-muted-foreground">{description}</p>
        <div className="flex flex-wrap gap-1.5">{children}</div>
      </CardContent>
    </Card>
  )
}
