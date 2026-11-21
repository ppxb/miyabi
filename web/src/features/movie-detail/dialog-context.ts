import { createContext } from 'react'

export type MovieDetailTarget = { id: string; code?: string } | { code: string } | { libraryId: number }

export function movieDetailKey(movie: MovieDetailTarget) {
  if ('libraryId' in movie) return `library:${movie.libraryId}`
  return 'id' in movie ? `id:${movie.id}` : `code:${movie.code}`
}

export const MovieDetailDialogContext = createContext<
  ((movie: MovieDetailTarget) => void) | null
>(null)
