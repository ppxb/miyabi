import { Link } from '@tanstack/react-router'
import { CheckIcon, CloudDownloadIcon, CopyIcon, LoaderCircleIcon } from 'lucide-react'
import { useState } from 'react'

import type { DiscoverMagnet, useDiscoverMagnets } from '@/api/discover'
import { ApiError } from '@/api/client'
import { useAddOffline, useOfflineTasks, type OfflineSubmission } from '@/api/offline'
import { usePanAccount } from '@/api/pan'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { formatSize } from '@/lib/format'

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
  const unauthorized = account.error instanceof ApiError && account.error.status === 401
  const connected = account.data?.connected === true && !unauthorized
  const accountID = connected ? (account.data?.account?.id ?? '') : ''
  const offline = useOfflineTasks(movieID, accountID)
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
        <div className="flex items-center gap-3 text-sm text-destructive">
          <span>离线任务状态读取失败，请重试。</span>
          <Button type="button" variant="outline" size="sm" onClick={() => void offline.refetch()}>
            重试
          </Button>
        </div>
      ) : null}
      {query.isPending ? (
        <div className="space-y-2">
          {Array.from({ length: 3 }, (_, index) => (
            <Skeleton key={index} className="h-30 rounded-2xl" />
          ))}
        </div>
      ) : query.isError ? (
        <div className="flex items-center gap-3 text-sm text-destructive">
          <span>磁力加载失败</span>
          <Button type="button" variant="outline" size="sm" onClick={() => query.refetch()}>
            重试
          </Button>
        </div>
      ) : query.data.length === 0 ? (
        <p className="text-sm text-muted-foreground">暂无磁力链</p>
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
      {copyError ? <p className="text-sm text-destructive">复制失败，请重试。</p> : null}
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
            <a
              href={magnet.uri}
              className="block font-mono text-xs break-all text-muted-foreground outline-ring select-all hover:text-foreground"
            >
              {magnet.uri}
            </a>
            <div className="flex flex-wrap items-center gap-x-4 gap-y-2 text-sm leading-6 text-muted-foreground">
              <span className="text-xs">{formatSize(magnet.size)}</span>
              {magnet.has_subtitle ? <Badge variant="outline">字幕</Badge> : null}
              {magnet.hd ? <Badge variant="outline">高清</Badge> : null}
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-2 self-end sm:self-center">
            <Button type="button" variant="outline" size="sm" onClick={onCopy}>
              {copied ? <CheckIcon /> : <CopyIcon />}
              {copied ? '已复制' : '复制'}
            </Button>
            {connected ? (
              <Button
                type="button"
                size="sm"
                disabled={!hasDirectory || busy || statusError || submitted}
                onClick={() => add.mutate(magnet.hash)}
              >
                {busy || task?.phase === 'downloading' || task?.phase === 'processing' ? (
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
        {error ? <p className="text-sm text-destructive">{error}</p> : null}
      </CardContent>
    </Card>
  )
}
