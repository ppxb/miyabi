import { createFileRoute } from '@tanstack/react-router'

import { MoviesPage } from '@/features/movies/page'

export const Route = createFileRoute('/movies')({
  component: MoviesPage
})
