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
  const checkingStatus = offline.isPending || !offline.isFetchedAfterMount
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
      <h2 id="movie-magnets-title" className="text-xl font-semibold tracking-normal">
        磁力链
      </h2>
      {connected && account.data?.account && !hasDirectory ? (
        <p role="status" className="text-sm text-muted-foreground">
          请先到{' '}
          <Link to="/settings" className="text-foreground underline underline-offset-4">
            设置页
          </Link>{' '}
          挂载媒体目录，再一键加入 115。
        </p>
      ) : null}
      {connected && offline.isError ? (
        <div role="alert" className="flex items-center gap-3 text-sm text-destructive">
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
              accountID={accountID}
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
      {copyError ? (
        <p role="alert" className="text-sm text-destructive">
          复制失败，请重试。
        </p>
      ) : null}
    </section>
  )
}

function MagnetCard({
  movieID,
  accountID,
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
  accountID: string
  magnet: DiscoverMagnet
  task?: OfflineSubmission
  checkingStatus: boolean
  statusError: boolean
  connected: boolean
  hasDirectory: boolean
  copied: boolean
  onCopy: () => void
}) {
  const add = useAddOffline(movieID, accountID)
  const submitted = task?.status === 'queued' || task?.status === 'running'
  const busy = add.isPending || (checkingStatus && !submitted)
  let label = '一键加入 115'
  if (add.isPending) {
    label = '提交中…'
  } else if (submitted) {
    label = '已加入 115'
  } else if (checkingStatus) {
    label = '读取状态…'
  } else if (statusError) {
    label = '状态暂不可用'
  } else if (task?.status === 'done' || task?.status === 'failed') {
    label = '重新加入 115'
  }

  let error: string | undefined
  if (add.isError) {
    error =
      add.error instanceof ApiError
        ? add.error.status === 401
          ? '115 授权已失效，请到设置页重新登录。'
          : add.error.message
        : '加入失败，请检查后端服务和网络后重试。'
  } else if (!add.isPending && task?.status === 'failed') {
    error = task.error
  }

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
                {busy ? (
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
        {error ? (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        ) : null}
      </CardContent>
    </Card>
  )
}

function formatSize(bytes: number) {
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let size = bytes
  let unit = 0
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024
    unit++
  }
  return `${new Intl.NumberFormat('zh-CN', { maximumFractionDigits: 2 }).format(size)} ${units[unit]}`
}
