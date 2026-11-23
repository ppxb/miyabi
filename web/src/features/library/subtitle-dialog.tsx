import { DownloadIcon, EyeIcon, EyeOffIcon, Trash2Icon } from 'lucide-react'
import { useScrollFade } from '@animate-ui/react/primitives/effects/scroll-fade'
import { useRef, useState, type PropsWithChildren } from 'react'

import { describeApiError } from '@/api/client'
import {
  useDeleteSubtitle,
  useMovieSubtitles,
  usePreviewSubtitle,
  useSaveSubtitle,
  useSubtitleSearch,
  subtitleLanguageLabel,
  subtitleVersionLabels,
  type SubtitleOffer,
  type SubtitlePreview,
  type SubtitleTrack
} from '@/api/subtitles'
import { InlineError } from '@/components/error-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle
} from '@/components/ui/dialog'
import { Spinner } from '@/components/ui/spinner'

const languageLabel = subtitleLanguageLabel
const versionLabels = subtitleVersionLabels

function formatTime(seconds: number) {
  const total = Math.floor(seconds)
  const h = Math.floor(total / 3600)
  const m = Math.floor((total % 3600) / 60)
  const s = total % 60
  const mmss = `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
  return h > 0 ? `${h}:${mmss}` : mmss
}

function KindBadges({
  language,
  version,
  format
}: {
  language: string
  version: string
  format: string
}) {
  return (
    <>
      <Badge variant="secondary">{languageLabel(language)}</Badge>
      {versionLabels[version] ? <Badge>{versionLabels[version]}</Badge> : null}
      <Badge>{format.toUpperCase()}</Badge>
    </>
  )
}

// Opens from the library card menu. Users pick subtitles themselves: online
// results are only listed, previewed on demand, and saved one at a time.
export function SubtitleDialog({
  movie,
  open,
  onOpenChange
}: {
  movie: { id: number; code: string }
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  // Keep the body mounted through the close animation so the whole panel fades
  // out together; unmounting it on `open=false` collapsed the dialog to an
  // empty shell mid-animation, which read as a visual residue.
  const [closing, setClosing] = useState(false)
  const showBody = open || closing

  return (
    <Dialog
      open={open}
      onOpenChange={next => {
        setClosing(!next)
        onOpenChange(next)
      }}
      onOpenChangeComplete={next => {
        if (!next) setClosing(false)
      }}
    >
      <DialogContent className="flex max-h-[85vh] flex-col overflow-hidden sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{movie.code} 的字幕</DialogTitle>
          <DialogDescription>预览确认内容与时长后再下载</DialogDescription>
        </DialogHeader>
        <ScrollBody active={open}>
          {showBody ? <SubtitleManager movieID={movie.id} /> : null}
        </ScrollBody>
      </DialogContent>
    </Dialog>
  )
}

/** Scrollable dialog body with the shared animated scroll fade; the header stays put. */
function ScrollBody({ children, active }: PropsWithChildren<{ active: boolean }>) {
  const ref = useRef<HTMLDivElement>(null)
  // Disable the fade (and clear its mask) while closing, so the mask layer does
  // not linger over the dialog's exit animation.
  useScrollFade(ref, { enabled: active })
  return (
    <div
      ref={ref}
      className="-mx-4 no-scrollbar min-h-0 flex-1 overflow-y-auto overscroll-contain px-4"
    >
      {children}
    </div>
  )
}

function SubtitleManager({ movieID }: { movieID: number }) {
  const tracks = useMovieSubtitles(movieID, true)
  const search = useSubtitleSearch(movieID, true)
  const remove = useDeleteSubtitle(movieID)
  const save = useSaveSubtitle(movieID)
  const preview = usePreviewSubtitle(movieID)
  const [previewing, setPreviewing] = useState<string | null>(null)

  const togglePreview = (offer: SubtitleOffer) => {
    if (previewing === offer.url) {
      setPreviewing(null)
      return
    }
    setPreviewing(offer.url)
    preview.mutate(offer)
  }

  return (
    <div className="space-y-6">
      <section className="space-y-2">
        <h3 className="font-medium">已有字幕</h3>
        {tracks.isPending ? (
          <Spinner />
        ) : tracks.isError ? (
          <InlineError onRetry={() => void tracks.refetch()} retrying={tracks.isFetching}>
            {describeApiError(tracks.error)}
          </InlineError>
        ) : tracks.data.length === 0 ? (
          <p className="text-muted-foreground">暂无字幕</p>
        ) : (
          <ul className="divide-y rounded-lg border">
            {tracks.data.map(track => (
              <TrackRow
                key={track.id}
                track={track}
                deleting={remove.isPending && remove.variables === track.id}
                onDelete={() => remove.mutate(track.id)}
              />
            ))}
          </ul>
        )}
      </section>

      <section className="space-y-2">
        <h3 className="font-medium">在线搜索</h3>
        {search.isPending ? (
          <p className="flex items-center gap-2 text-muted-foreground">
            <Spinner /> 正在搜索字幕来源…
          </p>
        ) : search.isError ? (
          <InlineError onRetry={() => void search.refetch()} retrying={search.isFetching}>
            {describeApiError(search.error)}
          </InlineError>
        ) : search.data.length === 0 ? (
          <p className="text-muted-foreground">未找到匹配的字幕</p>
        ) : (
          <ul className="divide-y rounded-lg border">
            {search.data.map(offer => (
              <li key={offer.url} className="space-y-2 p-3">
                <div className="flex flex-wrap items-center gap-2">
                  <Badge variant="frosted">{offer.provider}</Badge>
                  <KindBadges
                    language={offer.language}
                    version={offer.version}
                    format={offer.format}
                  />
                  <span
                    className="min-w-0 flex-1 truncate text-muted-foreground"
                    title={offer.name}
                  >
                    {offer.name}
                  </span>
                  <Button size="sm" variant="outline" onClick={() => togglePreview(offer)}>
                    {previewing === offer.url ? <EyeOffIcon /> : <EyeIcon />}
                    预览
                  </Button>
                  <Button size="sm" disabled={save.isPending} onClick={() => save.mutate(offer)}>
                    {save.isPending && save.variables?.url === offer.url ? (
                      <Spinner />
                    ) : (
                      <DownloadIcon />
                    )}
                    下载
                  </Button>
                </div>
                {previewing === offer.url ? (
                  preview.isPending || preview.variables?.url !== offer.url ? (
                    <Spinner />
                  ) : preview.isError ? (
                    <InlineError>{describeApiError(preview.error)}</InlineError>
                  ) : preview.data ? (
                    <PreviewPanel preview={preview.data} />
                  ) : null
                ) : null}
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  )
}

function TrackRow({
  track,
  deleting,
  onDelete
}: {
  track: SubtitleTrack
  deleting: boolean
  onDelete: () => void
}) {
  return (
    <li className="flex flex-wrap items-center gap-2 p-3">
      <KindBadges language={track.language} version={track.version} format={track.format} />
      <span className="min-w-0 flex-1 truncate text-muted-foreground" title={track.name}>
        {track.name}
      </span>
      {track.removable ? (
        <Button size="sm" variant="destructive" disabled={deleting} onClick={onDelete}>
          {deleting ? <Spinner /> : <Trash2Icon />}
          删除
        </Button>
      ) : (
        <span className="text-xs text-muted-foreground">本地文件</span>
      )}
    </li>
  )
}

function PreviewPanel({ preview }: { preview: SubtitlePreview }) {
  // A subtitle ending well before the movie is usually partial or for another cut.
  const short = preview.runtime > 0 && preview.end < preview.runtime * 60 * 0.8
  const linesRef = useRef<HTMLOListElement>(null)
  useScrollFade(linesRef)
  return (
    <div className="space-y-2 rounded-md bg-muted/50 p-3">
      <p className="text-xs text-muted-foreground">
        {languageLabel(preview.language)} · 共 {preview.cues} 条 · 最后一条{' '}
        {formatTime(preview.end)}
        {preview.runtime > 0 ? ` · 影片时长 ${preview.runtime} 分钟` : ''}
      </p>
      {short ? (
        <p className="text-xs text-destructive">
          字幕结束时间明显早于影片时长，可能不完整或不对应此版本。
        </p>
      ) : null}
      <ol
        ref={linesRef}
        className="no-scrollbar max-h-48 space-y-1 overflow-y-auto overscroll-contain text-xs"
      >
        {preview.lines.map((line, index) => (
          <li key={index} className="flex gap-3">
            <span className="shrink-0 text-muted-foreground tabular-nums">
              {formatTime(line.start)}
            </span>
            <span>{line.text}</span>
          </li>
        ))}
      </ol>
    </div>
  )
}
