import { CaptionsIcon, PencilIcon, PlayIcon, RefreshCwIcon } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'

import { describeApiError } from '@/api/client'
import { useRescrapeLibraryMovie, type LibraryMovie } from '@/api/library'
import { fetchMpvLink } from '@/api/play'
import { InlineError } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { ContextMenuContent, ContextMenuItem } from '@/components/ui/context-menu'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { SubtitleDialog } from '@/features/library/subtitle-dialog'
import { notifyTaskError, useNotifyScanTask } from '@/features/tasks/task-toast'

// Renders the context-menu content and its correction dialog. The ContextMenu
// root/trigger live in the card so right-clicking the whole card opens it.
export function LibraryMovieActions({ movie }: { movie: Pick<LibraryMovie, 'id' | 'code'> }) {
  const [correcting, setCorrecting] = useState(false)
  const [managingSubtitles, setManagingSubtitles] = useState(false)
  const [code, setCode] = useState(movie.code)
  const scrape = useRescrapeLibraryMovie(movie.id)
  const notifyScan = useNotifyScanTask()
  const busy = scrape.isPending
  const [launchingMpv, setLaunchingMpv] = useState(false)

  async function playWithMpv() {
    if (launchingMpv) return
    setLaunchingMpv(true)
    try {
      const { url } = await fetchMpvLink(movie.id)
      window.location.href = url
    } catch (error) {
      toast.error('无法唤起 MPV', { description: describeApiError(error) })
    } finally {
      setLaunchingMpv(false)
    }
  }

  return (
    <>
      <ContextMenuContent>
        <ContextMenuItem
          disabled={busy}
          onClick={() =>
            scrape.mutate(undefined, {
              onSuccess: notifyScan,
              onError: error =>
                notifyTaskError(
                  `movie:${movie.id}:submit-error`,
                  '无法创建刮削任务',
                  describeApiError(error)
                )
            })
          }
        >
          <RefreshCwIcon />
          重新刮削
        </ContextMenuItem>
        <ContextMenuItem
          disabled={busy}
          onClick={() => {
            scrape.reset()
            setCode(movie.code)
            setCorrecting(true)
          }}
        >
          <PencilIcon />
          纠正番号
        </ContextMenuItem>
        <ContextMenuItem onClick={() => setManagingSubtitles(true)}>
          <CaptionsIcon />
          字幕下载
        </ContextMenuItem>
        <ContextMenuItem disabled={launchingMpv} onClick={() => void playWithMpv()}>
          <PlayIcon />
          使用 MPV 播放
        </ContextMenuItem>
      </ContextMenuContent>
      <SubtitleDialog movie={movie} open={managingSubtitles} onOpenChange={setManagingSubtitles} />
      <Dialog
        open={correcting}
        onOpenChange={open => {
          if (!scrape.isPending) setCorrecting(open)
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>纠正番号</DialogTitle>
            <DialogDescription>
              当前番号：{movie.code}。保存后会重新刮削，并在后续同步与重建时沿用纠正结果。
            </DialogDescription>
          </DialogHeader>
          <form
            className="space-y-6"
            onSubmit={event => {
              event.preventDefault()
              if (busy || !code.trim() || code.trim() === movie.code) return
              scrape.mutate(code.trim(), {
                onSuccess: task => {
                  setCorrecting(false)
                  notifyScan(task)
                }
              })
            }}
          >
            <Input
              value={code}
              onChange={event => setCode(event.target.value)}
              maxLength={120}
              disabled={scrape.isPending}
              autoFocus
            />
            {scrape.error ? <InlineError>{describeApiError(scrape.error)}</InlineError> : null}
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                disabled={scrape.isPending}
                onClick={() => setCorrecting(false)}
              >
                取消
              </Button>
              <Button type="submit" disabled={busy || !code.trim() || code.trim() === movie.code}>
                {scrape.isPending ? '提交中…' : '保存并重新刮削'}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </>
  )
}
