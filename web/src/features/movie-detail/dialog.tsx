import { useScrollFade } from '@animate-ui/react/primitives/effects/scroll-fade'
import { useRouter } from '@tanstack/react-router'
import {
  lazy,
  Suspense,
  useCallback,
  useEffect,
  useRef,
  useState,
  type PropsWithChildren
} from 'react'

import { Dialog, DialogContent, DialogTitle } from '@/components/ui/dialog'
import { MovieDetailDialogContext, movieDetailKey, type MovieDetailTarget } from './dialog-context'
import { MovieDetailSkeleton } from './skeleton'

const MovieDetailContent = lazy(() =>
  import('./content').then(module => ({ default: module.MovieDetailContent }))
)

/** Scrollable detail body with the shared animated scroll fade. */
function DetailScrollBody({ children }: PropsWithChildren) {
  const ref = useRef<HTMLDivElement>(null)
  useScrollFade(ref, { shrink: 0, blur: 0 })
  return (
    <div
      ref={ref}
      className="no-scrollbar min-h-0 flex-1 overflow-y-auto overscroll-contain p-4 sm:p-6"
    >
      {children}
    </div>
  )
}

export function MovieDetailDialogProvider({ children }: PropsWithChildren) {
  const router = useRouter()
  const [movie, setMovie] = useState<MovieDetailTarget>()
  const [open, setOpen] = useState(false)
  const openMovie = useCallback((target: MovieDetailTarget) => {
    setMovie(target)
    setOpen(true)
  }, [])

  useEffect(() => router.subscribe('onBeforeNavigate', () => setOpen(false)), [router])

  return (
    <MovieDetailDialogContext value={openMovie}>
      {children}
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent
          showCloseButton={false}
          className="flex h-[min(56rem,calc(100dvh-2rem))] w-[calc(100%-2rem)] flex-col gap-0 overflow-hidden p-0 sm:max-w-6xl"
        >
          <DialogTitle className="sr-only">影片详情</DialogTitle>
          <DetailScrollBody key={movie && movieDetailKey(movie)}>
            <Suspense fallback={<MovieDetailSkeleton />}>
              {movie ? <MovieDetailContent movie={movie} /> : null}
            </Suspense>
          </DetailScrollBody>
        </DialogContent>
      </Dialog>
    </MovieDetailDialogContext>
  )
}
