import { createContext } from 'react'

export const MovieDetailDialogContext = createContext<
  ((movieId: string, trigger: HTMLDivElement) => void) | null
>(null)
