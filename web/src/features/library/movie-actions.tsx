import { Ellipsis, PencilIcon, RefreshCwIcon } from 'lucide-react'
import { useRef, useState } from 'react'

import { describeApiError } from '@/api/client'
import { useRescrapeLibraryMovie, type LibraryMovie } from '@/api/library'
import { InlineError } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { notifyTaskError, useNotifyScanTask } from '@/features/tasks/task-toast'

export function LibraryMovieActions({
  movie,
  disabled = false
}: {
  movie: LibraryMovie
  disabled?: boolean
}) {
  const [correcting, setCorrecting] = useState(false)
  const [code, setCode] = useState(movie.code)
  const trigger = useRef<HTMLButtonElement>(null)
  const scrape = useRescrapeLibraryMovie(movie.id)
  const notifyScan = useNotifyScanTask()
  const busy = disabled || scrape.isPending

  return (
    <Dialog
      open={correcting}
      onOpenChange={open => {
        if (!scrape.isPending) setCorrecting(open)
      }}
    >
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button ref={trigger} type="button" variant="ghost" size="icon-sm" disabled={busy}>
            <Ellipsis />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent
          align="end"
          onCloseAutoFocus={event => {
            if (correcting) event.preventDefault()
          }}
        >
          <DropdownMenuItem
            disabled={busy}
            onSelect={() =>
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
          </DropdownMenuItem>
          <DropdownMenuItem
            disabled={busy}
            onSelect={() => {
              scrape.reset()
              setCode(movie.code)
              setCorrecting(true)
            }}
          >
            <PencilIcon />
            纠正番号
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <DialogContent
        onCloseAutoFocus={event => {
          event.preventDefault()
          trigger.current?.focus()
        }}
      >
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
  )
}
