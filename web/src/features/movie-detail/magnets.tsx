import { Link } from '@tanstack/react-router'
import {
  CheckIcon,
  CloudDownloadIcon,
  CopyIcon,
  LoaderCircleIcon,
  SkipForwardIcon
} from 'lucide-react'
import { useState } from 'react'

import type { DiscoverMagnet, useDiscoverMagnets } from '@/api/discover'
import { describeApiError, isPanUnauthorized } from '@/api/client'
import {
  isOfflineTaskActive,
  useAddOffline,
  useOfflineTasks,
  useOfflineControl,
  type OfflineSubmission
} from '@/api/offline'
import { usePanAccount } from '@/api/pan'
import { InlineError } from '@/components/error-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { formatSize } from '@/lib/format'
import { notifyOfflineTask, notifyTaskError } from '@/features/tasks/task-toast'
import { MovieSubscriptionAction } from './subscription-action'
import { MovieMagnetsSkeleton } from './skeleton'

const sourceLabels: Record<string, string> = { javdb: 'JavDB', javbus: 'JavBus' }

const phaseLabels: Record<OfflineSubmission['phase'], string> = {
  available: '一键加入 115',
  downloading: '下载中',
  processing: '入库处理中',
  in_library: '已入库',
  downloaded: '已下载'
}

export function MovieMagnets({
  movieID,
  query
}: {
  movieID: string
  query: ReturnType<typeof useDiscoverMagnets>
}) {
  const account = usePanAccount(Boolean(query.data?.length))
  const unauthorized = isPanUnauthorized(account.error)
  const connected = account.data?.connected === true && !unauthorized
  const accountID = connected ? (account.data?.account?.id ?? '') : ''
  const offline = useOfflineTasks(movieID, accountID, account.data?.directory?.id ?? '')
  const tasks = new Map(offline.data?.map(task => [task.hash, task] as const))
  const checkingStatus = offline.isPending
  const hasDirectory = Boolean(account.data?.directory)
  const [copiedHash, setCopiedHash] = useState('')
  const [copyError, setCopyError] = useState(false)

  async function copy(magnet: DiscoverMagnet) {
    setCopyError(false)
    try {
      await navigator.clipboard.writeText(magnet.uri)
      setCopiedHash(magnet.hash)
    } catch {
      setCopyError(true)
    }
  }

  return (
    <section className="space-y-4">
      <h2 className="text-xl font-semibold tracking-normal">磁力链</h2>
      {connected && account.data?.account && !hasDirectory ? (
        <p className="text-sm text-muted-foreground">
          请先到{' '}
          <Link to="/settings" className="text-foreground underline underline-offset-4">
            设置页
          </Link>{' '}
          挂载媒体目录，再一键加入 115。
        </p>
      ) : null}
      {connected && offline.isError ? (
        <InlineError onRetry={() => void offline.refetch()} retrying={offline.isFetching}>
          离线任务状态读取失败，请重试。
        </InlineError>
      ) : null}
      {query.isPending ? (
        <MovieMagnetsSkeleton />
      ) : query.isError ? (
        <InlineError onRetry={() => void query.refetch()} retrying={query.isFetching}>
          磁力加载失败
        </InlineError>
      ) : query.data.length === 0 ? (
        <MovieSubscriptionAction movieID={movieID} />
      ) : (
        <div className="space-y-4">
          {query.data.map(magnet => (
            <MagnetCard
              key={`${movieID}:${magnet.hash}`}
              movieID={movieID}
              magnet={magnet}
              task={tasks.get(magnet.hash)}
              checkingStatus={checkingStatus}
              statusError={offline.isError}
              connected={connected}
              hasDirectory={hasDirectory}
              copied={copiedHash === magnet.hash}
              onCopy={() => void copy(magnet)}
            />
          ))}
        </div>
      )}
      {copyError ? <InlineError>复制失败，请重试。</InlineError> : null}
    </section>
  )
}

function MagnetCard({
  movieID,
  magnet,
  task,
  checkingStatus,
  statusError,
  connected,
  hasDirectory,
  copied,
  onCopy
}: {
  movieID: string
  magnet: DiscoverMagnet
  task?: OfflineSubmission
  checkingStatus: boolean
  statusError: boolean
  connected: boolean
  hasDirectory: boolean
  copied: boolean
  onCopy: () => void
}) {
  const add = useAddOffline(movieID)
  const next = useOfflineControl('next')
  const submitted = task !== undefined && task.phase !== 'available'
  const busy = add.isPending || (checkingStatus && !submitted)
  let label = '一键加入 115'
  if (add.isPending) {
    label = '提交中'
  } else if (submitted) {
    label = phaseLabels[task.phase]
  } else if (checkingStatus) {
    label = '读取状态'
  } else if (statusError) {
    label = '状态暂不可用'
  }

  const error = !add.isPending ? task?.error : undefined

  return (
    <Card size="sm">
      <CardContent className="space-y-3">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-center">
          <div className="min-w-0 flex-1 space-y-2">
            <h3 className="text-base leading-6 font-semibold wrap-break-word">{magnet.name}</h3>
            <p className="w-fit max-w-full font-mono text-xs break-all text-muted-foreground select-all">
              {magnet.uri}
            </p>
            <div className="flex flex-wrap items-center gap-x-4 gap-y-2 text-sm leading-6 text-muted-foreground">
              <span className="text-xs">{formatSize(magnet.size)}</span>
              {magnet.sources?.map(source => (
                <Badge key={source} variant="outline">
                  {sourceLabels[source] ?? source}
                </Badge>
              ))}
              {magnet.tags?.map(tag => (
                <Badge key={tag} variant="outline">
                  {tag}
                </Badge>
              ))}
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-2 self-end sm:self-center">
            {task?.can_switch ? (
              <Button
                type="button"
                variant="outline"
                size="icon-sm"
                title="尝试下一条磁力"
                disabled={next.isPending || statusError}
                onClick={() => next.mutate(task.task_id)}
              >
                {next.isPending ? (
                  <LoaderCircleIcon className="animate-spin" />
                ) : (
                  <SkipForwardIcon />
                )}
              </Button>
            ) : null}
            <Button type="button" variant="outline" size="sm" onClick={onCopy}>
              {copied ? <CheckIcon /> : <CopyIcon />}
              {copied ? '已复制' : '复制'}
            </Button>
            {connected ? (
              <Button
                type="button"
                size="sm"
                disabled={!hasDirectory || busy || statusError || submitted}
                onClick={() =>
                  add.mutate(magnet.hash, {
                    onSuccess: submission => {
                      notifyOfflineTask(submission)
                    },
                    onError: error => {
                      notifyTaskError(
                        `offline:submit-error:${movieID}`,
                        '加入 115 失败',
                        describeApiError(error)
                      )
                    }
                  })
                }
              >
                {busy || (task && isOfflineTaskActive(task)) ? (
                  <LoaderCircleIcon className="animate-spin" />
                ) : submitted ? (
                  <CheckIcon />
                ) : (
                  <CloudDownloadIcon />
                )}
                {label}
              </Button>
            ) : null}
          </div>
        </div>
        {task?.phase === 'in_library' && task.processing ? (
          <p className="text-sm text-muted-foreground">文件已入库，后台整理中。</p>
        ) : null}
        {task && (task.attempt_count ?? 0) > 1 ? (
          <p className="text-xs text-muted-foreground">
            已尝试 {task.attempt_count} 条磁力{task.switch_reason ? ` · ${task.switch_reason}` : ''}
          </p>
        ) : null}
        {error ? <InlineError>{error}</InlineError> : null}
      </CardContent>
    </Card>
  )
}
