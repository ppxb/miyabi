import { createContext } from 'react'

export const MovieDetailDialogContext = createContext<
  ((movieId: string, trigger: HTMLAnchorElement) => void) | null
>(null)
