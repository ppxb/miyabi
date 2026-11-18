import { HlsVideo } from '@videojs/react/media/hls-video'
import { VideoPlayer, VideoSkin } from '@videojs/react/video'
import { StarIcon } from 'lucide-react'
import { useMemo, type ReactNode } from 'react'

import { useLibraryMovie } from '@/api/library'
import { usePlayback, usePlayFiles } from '@/api/play'
import { Badge } from '@/components/ui/badge'
import { Dialog, DialogContent, DialogTitle } from '@/components/ui/dialog'

import '@videojs/react/video/skin.css'

export function LibraryPlayerDialog({
  movieId,
  onClose
}: {
  movieId: number | null
  onClose: () => void
}) {
  if (movieId === null) return null
  return (
    <Dialog open onOpenChange={open => (open ? undefined : onClose())}>
      <DialogContent
        showCloseButton={false}
        className="flex h-[min(54rem,calc(100dvh-2rem))] w-[calc(100%-2rem)] max-w-[96rem] flex-col gap-0 overflow-hidden p-0 sm:max-w-[96rem] lg:flex-row"
      >
        <LibraryPlayer movieId={movieId} />
      </DialogContent>
    </Dialog>
  )
}

function LibraryPlayer({ movieId }: { movieId: number }) {
  const detail = useLibraryMovie(movieId)
  const files = usePlayFiles(movieId)
  const file = files.data?.files[0]
  const playback = usePlayback(file?.id)
  const source = useMemo(() => {
    const sources = playback.data?.sources ?? []
    return sources.find(item => item.label.startsWith('原画')) ?? sources[0]
  }, [playback.data])

  const title = detail.data?.title || detail.data?.code || '播放'
  const error = !file || playback.isError || files.isError

  return (
    <>
      <DialogTitle className="sr-only">{title}</DialogTitle>
      <div className="flex min-w-0 flex-1 items-center justify-center bg-black">
        {source && !error ? (
          <VideoPlayer title={title}>
            <VideoSkin style={{ aspectRatio: '16 / 9', width: '100%' }}>
              <HlsVideo src={source.src} playsInline autoPlay />
            </VideoSkin>
          </VideoPlayer>
        ) : (
          <div className="text-sm text-white/70">
            {error ? '播放地址加载失败' : '正在加载播放地址…'}
          </div>
        )}
      </div>
      <aside className="min-w-0 shrink-0 overflow-y-auto border-t bg-background p-5 lg:w-[26rem] lg:border-t-0 lg:border-l">
        <MovieInfo movieId={movieId} />
      </aside>
    </>
  )
}

function MovieInfo({ movieId }: { movieId: number }) {
  const detail = useLibraryMovie(movieId)
  const movie = detail.data

  if (!movie) {
    return <p className="text-sm text-muted-foreground">正在加载影片信息…</p>
  }

  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant="outline" className="tabular-nums">
            {movie.code}
          </Badge>
          {movie.rating > 0 ? (
            <span className="inline-flex items-center gap-1 text-sm font-semibold text-amber-500">
              <StarIcon className="size-4 fill-amber-400 text-amber-400" />
              <span className="tabular-nums">{movie.rating.toFixed(1)}</span>
            </span>
          ) : null}
        </div>
        <h2 className="text-xl leading-snug font-bold">{movie.title || movie.code}</h2>
        {movie.origin_title && movie.origin_title !== movie.title ? (
          <p className="text-sm text-muted-foreground">{movie.origin_title}</p>
        ) : null}
      </div>

      <dl className="grid grid-cols-2 gap-3 text-sm">
        {movie.release_date ? (
          <div>
            <dt className="text-xs text-muted-foreground">发行日期</dt>
            <dd className="tabular-nums">{movie.release_date}</dd>
          </div>
        ) : null}
        {movie.duration > 0 ? (
          <div>
            <dt className="text-xs text-muted-foreground">片长</dt>
            <dd className="tabular-nums">{movie.duration} 分钟</dd>
          </div>
        ) : null}
        {movie.maker?.name ? (
          <div>
            <dt className="text-xs text-muted-foreground">厂商</dt>
            <dd>{movie.maker.name}</dd>
          </div>
        ) : null}
        {movie.series?.name ? (
          <div>
            <dt className="text-xs text-muted-foreground">系列</dt>
            <dd>{movie.series.name}</dd>
          </div>
        ) : null}
      </dl>

      <InfoRow label="演员">
        {movie.actors.length > 0 ? (
          <span className="flex flex-wrap gap-x-2 gap-y-1">
            {movie.actors.map(actor => (
              <span key={`${actor.provider ?? ''}:${actor.id}`}>{actor.name}</span>
            ))}
          </span>
        ) : (
          <span className="text-muted-foreground">N/A</span>
        )}
      </InfoRow>

      <InfoRow label="标签">
        {movie.tags.length > 0 ? (
          <span className="flex flex-wrap gap-1.5">
            {movie.tags.map(tag => (
              <Badge key={`${tag.provider}:${tag.id}`} variant="outline">
                {tag.name}
              </Badge>
            ))}
          </span>
        ) : (
          <span className="text-muted-foreground">N/A</span>
        )}
      </InfoRow>
    </div>
  )
}

function InfoRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-1">
      <p className="text-xs text-muted-foreground">{label}</p>
      <div className="text-sm leading-6">{children}</div>
    </div>
  )
}
