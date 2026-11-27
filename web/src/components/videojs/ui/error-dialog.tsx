'use client'

import '../styles/base.css'
import '../styles/audio/theme.css'
import '../styles/video/captions.css'
import '../styles/video/theme.css'
import { ErrorDialog as ErrorDialogPrimitive } from '@videojs/react'
import { usePlayer } from '@videojs/react/video'
import { useState } from 'react'
import { toast } from 'sonner'

import { describeApiError } from '@/api/client'
import { fetchMpvLink } from '@/api/play'
import { Button } from '@/components/videojs/ui/button'
import { cn } from '@/lib/resolve-class-name'

export function ErrorDialog({ movieId }: { movieId?: number }) {
  const error = usePlayer(state => state.error)
  const [launching, setLaunching] = useState(false)
  // Decode (3) and unsupported-source/format (4) errors are exactly the cases
  // where the local MPV player is the useful fallback.
  const undecodable = error != null && (error.code === 3 || error.code === 4)

  async function playWithMpv() {
    if (launching || movieId === undefined) return
    setLaunching(true)
    try {
      const { url } = await fetchMpvLink(movieId)
      window.location.href = url
    } catch (err) {
      toast.error('无法唤起 MPV', { description: describeApiError(err) })
    } finally {
      setLaunching(false)
    }
  }

  return (
    <ErrorDialogPrimitive.Root>
      <ErrorDialogPrimitive.Backdrop
        className={cn(
          'absolute inset-0 z-40 rounded-[inherit] bg-media-backdrop/20 opacity-100 backdrop-filter-media-dialog',
          'transition-opacity delay-media-dialog duration-media-dialog ease-out not-data-open:hidden',
          'data-ending-style:delay-0 media-transitioning:opacity-0',
          'data-[scope=document]:fixed data-[scope=document]:rounded-none',
          'data-ending-style:duration-media-slower'
        )}
      />
      <ErrorDialogPrimitive.Popup
        className={cn(
          'absolute top-1/2 left-1/2 z-50 flex max-h-[calc(100%-0.5rem)] w-media-dialog-width max-w-media-dialog -translate-x-1/2 -translate-y-1/2 flex-col gap-3 rounded-media-dialog text-media-popover-foreground outline-hidden not-data-open:hidden',
          'transition-[opacity,scale] delay-media-dialog duration-media-dialog ease-out text-shadow-media-dialog',
          'data-ending-style:delay-0 media-transitioning:scale-media-hidden-popup media-transitioning:opacity-0',
          'data-[scope=document]:fixed data-[scope=document]:max-h-[calc(100dvh-1rem)]',
          'bg-media-popover surface-media after:surface-media-inset',
          'p-3',
          'data-ending-style:duration-media-slower'
        )}
      >
        <div className={cn('flex min-h-0 flex-col gap-2 overflow-y-auto', 'px-2 pt-2 pb-1.5')}>
          {undecodable ? (
            <ErrorDialogPrimitive.Title className={'m-0 text-media-lg leading-tight font-semibold'}>
              在线播放器解码失败
            </ErrorDialogPrimitive.Title>
          ) : (
            <ErrorDialogPrimitive.Title
              className={'m-0 text-media-lg leading-tight font-semibold'}
            />
          )}
          {undecodable ? (
            <ErrorDialogPrimitive.Description className={'m-0 wrap-anywhere opacity-70'}>
              当前视频的编码浏览器无法解码，请使用本地 MPV 播放器观看。
            </ErrorDialogPrimitive.Description>
          ) : (
            <ErrorDialogPrimitive.Description className={'m-0 wrap-anywhere opacity-70'} />
          )}
        </div>
        <div className={'flex shrink-0 gap-2'}>
          {undecodable && movieId !== undefined ? (
            <Button
              type="button"
              disabled={launching}
              onClick={() => void playWithMpv()}
              className={
                'h-media-control w-full flex-1 bg-media-primary! px-4 py-2 font-medium text-media-primary-foreground!'
              }
            >
              {launching ? '正在唤起…' : '使用 MPV 播放'}
            </Button>
          ) : null}
          <ErrorDialogPrimitive.Close
            render={<Button />}
            className={
              'h-media-control w-full flex-1 bg-media-primary! px-4 py-2 font-medium text-media-primary-foreground!'
            }
          />
        </div>
      </ErrorDialogPrimitive.Popup>
    </ErrorDialogPrimitive.Root>
  )
}
