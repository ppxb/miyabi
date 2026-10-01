import { createContext } from 'react'

export type MovieDetailTarget = { id: string } | { code: string }

export const MovieDetailDialogContext = createContext<
  ((movie: MovieDetailTarget, trigger: HTMLDivElement) => void) | null
>(null)
