import { useQuery } from '@tanstack/react-query'

import { apiPost } from '@/api/client'
import {
  createMovieStateLoader,
  movieStateOptions,
  type MovieIdentity,
  type MovieStateResult
} from '@/api/movie-state-cache'

const loadMovieState = createMovieStateLoader(movies =>
  apiPost<MovieStateResult[]>('/api/discover/movie-states', { movies })
)

export function useMovieState(movie?: MovieIdentity) {
  return useQuery(movieStateOptions(loadMovieState, movie)).data
}
