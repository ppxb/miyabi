import { Link } from '@tanstack/react-router'
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
        state={<MovieStateBadge movie={movie} />}
      >
        <MovieResourceBadges movie={movie} />
      </MovieCard>
    </Link>
  )
}

export function MovieCard({
  movie,
  description,
  coverLoading = 'lazy',
  onCoverReady,
  coverOverlay,
  titleTooltip = true,
  state,
  children
}: {
  movie: { code: string; title: string; cover?: string }
  description?: ReactNode
  coverLoading?: 'eager' | 'lazy'
  onCoverReady?: () => void
  coverOverlay?: ReactNode
  titleTooltip?: boolean
  state?: ReactNode
  children?: ReactNode
}) {
  const title = movie.title || movie.code
  const heading = <h3 className="truncate text-sm leading-5 font-semibold">{title}</h3>
  return (
    <Card size="sm" className="h-full gap-0 overflow-hidden py-0">
      <div className="relative flex aspect-3/2 items-center justify-center overflow-hidden bg-muted">
        <div className="absolute inset-0">
          <MovieCover source={movie.cover ?? ''} loading={coverLoading} onReady={onCoverReady} />
        </div>
        <div className="absolute top-2 left-2 flex max-w-[calc(100%-1rem)] flex-wrap gap-1.5">
          <Badge variant="outline" className="max-w-full truncate bg-background/85 backdrop-blur">
            {movie.code}
          </Badge>
          {state}
        </div>
        {coverOverlay}
      </div>
      <CardContent className="min-w-0 space-y-2 p-3">
        {titleTooltip ? <OverflowTooltip content={title}>{heading}</OverflowTooltip> : heading}
        {description != null ? (
          <div className="text-xs text-muted-foreground">{description}</div>
        ) : null}
        {children ? <div className="flex flex-wrap gap-1.5">{children}</div> : null}
      </CardContent>
    </Card>
  )
}
