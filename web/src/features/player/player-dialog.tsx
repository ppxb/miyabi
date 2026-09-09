import { lazy, Suspense } from 'react'

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle
} from '@/components/ui/dialog'
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
        className="max-h-[92dvh] gap-4 overflow-y-auto rounded-3xl p-4 sm:max-w-5xl sm:p-6"
        onPointerDownOutside={event => event.preventDefault()}
      >
        <DialogHeader className="min-w-0 pr-8">
          <DialogTitle className="truncate">{code} · 播放</DialogTitle>
          <DialogDescription>可选择原文件播放或 115 转码。</DialogDescription>
        </DialogHeader>
        <Suspense fallback={<PlayerLoading />}>
          <MoviePlayer key={code} code={code} />
        </Suspense>
      </DialogContent>
    </Dialog>
  )
}
