import type { ReactNode } from 'react'

import type { DiscoverMovie } from '@/api/discover'
import { DiscoverMovieCard } from '@/components/movie/movie-card'

export function MovieGrid({ movies }: { movies: DiscoverMovie[] }) {
  return (
    <MovieGridLayout>
      {movies.map(movie => (
        <DiscoverMovieCard key={movie.id} movie={movie} />
      ))}
    </MovieGridLayout>
  )
}

export function MovieGridLayout({ children }: { children: ReactNode }) {
  return (
    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 sm:gap-4 lg:grid-cols-3 lg:gap-6 xl:grid-cols-4">
      {children}
    </div>
  )
}
