import { lazy, Suspense } from 'react'

const loadLibraryPlayer = () => import('@/features/library/player-dialog')

/** Warm the player chunk when the user shows intent (hover or keyboard focus). */
export function preloadLibraryPlayer() {
  void loadLibraryPlayer().catch(() => {})
}

const LibraryPlayerDialog = lazy(() =>
  loadLibraryPlayer().then(module => ({ default: module.LibraryPlayerDialog }))
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
