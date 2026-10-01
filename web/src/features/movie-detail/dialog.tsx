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

export function MovieDetailDialogProvider({ children }: PropsWithChildren) {
  const router = useRouter()
  const [movie, setMovie] = useState<MovieDetailTarget>()
  const [open, setOpen] = useState(false)
  const triggerRef = useRef<HTMLDivElement | null>(null)
  const contentRef = useRef<HTMLDivElement>(null)
  const openMovie = useCallback(
    (target: MovieDetailTarget, trigger: HTMLDivElement) => {
      // Recommendations replace the detail while retaining the original list's focus target.
      if (!open) triggerRef.current = trigger
      setMovie(target)
      setOpen(true)
    },
    [open]
  )

  useEffect(() => router.subscribe('onBeforeNavigate', () => setOpen(false)), [router])

  return (
    <MovieDetailDialogContext value={openMovie}>
      {children}
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent
          ref={contentRef}
          showCloseButton={false}
          className="flex h-[min(56rem,calc(100dvh-2rem))] w-[calc(100%-2rem)] flex-col gap-0 overflow-hidden p-0 sm:max-w-6xl"
          aria-describedby={undefined}
          onOpenAutoFocus={event => {
            event.preventDefault()
            contentRef.current?.focus()
          }}
          onCloseAutoFocus={event => {
            event.preventDefault()
            triggerRef.current?.focus({ preventScroll: true })
          }}
        >
          <DialogTitle className="sr-only">影片详情</DialogTitle>
          <div
            key={movie && movieDetailKey(movie)}
            className="min-h-0 flex-1 scroll-fade scrollbar-none overflow-y-auto overscroll-contain p-4 sm:p-6 [&::-webkit-scrollbar]:hidden"
          >
            <Suspense fallback={<MovieDetailSkeleton />}>
              {movie ? <MovieDetailContent movie={movie} /> : null}
            </Suspense>
          </div>
        </DialogContent>
      </Dialog>
    </MovieDetailDialogContext>
  )
}
