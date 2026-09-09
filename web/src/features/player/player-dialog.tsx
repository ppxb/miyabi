import { lazy, Suspense } from 'react'

import { Dialog, DialogContent } from '@/components/ui/dialog'
import { useUIStore } from '@/stores/ui'
import { PlayerLoading } from './player-status'

const MoviePlayer = lazy(() => import('./movie-player'))

export function PlayerDialog() {
  const code = useUIStore(state => state.playbackCode)
  const close = useUIStore(state => state.closePlayer)

  // Unmount the portal with the player so the exit animation cannot show a collapsed, empty frame.
  if (code === null) return null

  return (
    <Dialog
      open
      onOpenChange={open => {
        if (!open) close()
      }}
    >
      <DialogContent
        className="dark aspect-video w-[min(72rem,calc(100vw-2rem),calc(88dvh*16/9))] max-w-none gap-0 overflow-hidden rounded-2xl bg-black p-0 text-foreground ring-0 sm:max-w-none"
        showCloseButton={false}
        onPointerDownOutside={event => event.preventDefault()}
      >
        <Suspense fallback={<PlayerLoading />}>
          <MoviePlayer key={code} code={code} />
        </Suspense>
      </DialogContent>
    </Dialog>
  )
}
