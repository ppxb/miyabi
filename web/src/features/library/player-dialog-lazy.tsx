import { lazy, Suspense } from 'react'

const LibraryPlayerDialog = lazy(() =>
  import('@/features/library/player-dialog').then(module => ({
    default: module.LibraryPlayerDialog
  }))
)

/** Shared lazy wrapper so every page mounts the player the same way. */
export function LazyLibraryPlayerDialog({
  movieId,
  onClose
}: {
  movieId: number | null
  onClose: () => void
}) {
  if (movieId === null) return null
  return (
    <Suspense fallback={null}>
      <LibraryPlayerDialog movieId={movieId} onClose={onClose} />
    </Suspense>
  )
}
