import type { LinkProps } from '@tanstack/react-router'
import type { ReactNode } from 'react'

import { LibraryMovieCard, type LibraryMovieCardData } from '@/features/library/movie-card'
import { MovieShelf } from '@/features/library/movie-shelf'

/** Fixed-width wrapper for an item in a horizontal shelf. */
export function MovieShelfItem({ children }: { children: ReactNode }) {
  return (
    <div className="w-[72vw] shrink-0 snap-start sm:w-[calc((100%-1rem)/2)] lg:w-[calc((100%-3rem)/3)] xl:w-[calc((100%-4.5rem)/4)]">
      {children}
    </div>
  )
}

/** Presentational horizontal shelf of movie cards; shelves only supply data. */
export function MovieShelfSection({
  title,
  movies,
  onPlay,
  href
}: {
  title: string
  movies: LibraryMovieCardData[]
  onPlay: (movieID: number) => void
  href?: LinkProps['to']
}) {
  return (
    <MovieShelf title={title} itemCount={movies.length} href={href}>
      {movies.map(movie => (
        <MovieShelfItem key={movie.id}>
          <LibraryMovieCard movie={movie} onPlay={() => onPlay(movie.id)} />
        </MovieShelfItem>
      ))}
    </MovieShelf>
  )
}
