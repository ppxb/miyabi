import { DatabaseIcon, RefreshCwIcon, Trash2Icon } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { toast } from 'sonner'

import { describeApiError } from '@/api/client'
import { useClearCache, useDataInfo, useLogs } from '@/api/data'
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
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { formatSize } from '@/lib/format'
import { SettingRow, SettingsSection } from './shared'

const numberFormat = new Intl.NumberFormat('zh-CN')

export function DataSection() {
  const info = useDataInfo()
  const logs = useLogs()
  const clearCache = useClearCache()
  const [confirmOpen, setConfirmOpen] = useState(false)
  const cache = info.data?.cache
  const logRef = useRef<HTMLPreElement>(null)

  // Keep the newest lines in view as the log refreshes.
  useEffect(() => {
    const el = logRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [logs.data])

  function openConfirmation() {
    clearCache.reset()
    setConfirmOpen(true)
  }

  function clearUnusedCache() {
    clearCache.mutate(undefined, {
      onSuccess: () => {
        setConfirmOpen(false)
        toast.success('未使用的图片缓存已清理')
      }
    })
  }

  return (
    <SettingsSection icon={<DatabaseIcon className="size-4" />} title="数据与缓存">
      <SettingRow
        title="数据目录"
        description={info.data?.data_directory ?? '无法读取数据目录'}
        descriptionLoading={info.isPending}
      >
        <Button
          type="button"
          variant="outline"
          size="icon"
          title="刷新统计"
          disabled={info.isFetching || clearCache.isPending}
          onClick={() => void info.refetch()}
        >
          {info.isFetching ? <Spinner /> : <RefreshCwIcon />}
        </Button>
      </SettingRow>
      <SettingRow title="数据库" description="包含媒体库索引与应用设置">
        {info.isPending ? (
          <Skeleton className="h-5 w-24" />
        ) : (
          <span className="text-sm font-medium tabular-nums">
            {info.data ? formatSize(info.data.database_size_bytes) : '—'}
          </span>
        )}
      </SettingRow>
      <SettingRow title="运行日志" description={logs.data?.path ?? '正在读取日志路径…'}>
        <Button
          type="button"
          variant="outline"
          size="icon"
          title="刷新日志"
          disabled={logs.isFetching}
          onClick={() => void logs.refetch()}
        >
          {logs.isFetching ? <Spinner /> : <RefreshCwIcon />}
        </Button>
      </SettingRow>
      <div className="overflow-hidden rounded-xl border bg-muted/30">
        <pre
          ref={logRef}
          className="h-64 overflow-auto p-3 font-sans text-xs leading-5 break-words whitespace-pre-wrap text-muted-foreground"
        >
          {logs.isPending
            ? '正在读取日志…'
            : logs.data?.lines.length
              ? logs.data.lines.join('\n')
              : '暂无日志'}
        </pre>
      </div>
      {logs.isError ? (
        <InlineError onRetry={() => void logs.refetch()} retrying={logs.isFetching}>
          无法读取运行日志。
        </InlineError>
      ) : null}
      <SettingRow
        title="图片缓存"
        description={
          cache
            ? `共 ${numberFormat.format(cache.entry_count)} 个文件（${formatSize(cache.size_bytes)}），可清理 ${numberFormat.format(cache.unused_entry_count)} 个（${formatSize(cache.unused_size_bytes)}）`
            : '统计本地保存的封面与缩略图'
        }
      >
        <Button
          type="button"
          variant="destructive"
          disabled={
            clearCache.isPending || info.isError || !cache || cache.unused_entry_count === 0
          }
          onClick={openConfirmation}
        >
          {clearCache.isPending ? (
            <Spinner data-icon="inline-start" />
          ) : (
            <Trash2Icon data-icon="inline-start" />
          )}
          清理未使用缓存
        </Button>
      </SettingRow>
      {info.isError ? (
        <InlineError onRetry={() => void info.refetch()} retrying={info.isFetching}>
          无法读取数据与缓存统计，请稍后重试。
        </InlineError>
      ) : null}
      <Dialog
        open={confirmOpen}
        onOpenChange={open => {
          if (!clearCache.isPending) setConfirmOpen(open)
        }}
      >
        <DialogContent showCloseButton={!clearCache.isPending}>
          <DialogHeader>
            <DialogTitle>清理未使用的图片缓存？</DialogTitle>
            <DialogDescription>
              仅删除未被影片或未完成任务引用的图片。媒体库封面和 115 网盘文件会保留。
            </DialogDescription>
          </DialogHeader>
          {clearCache.error ? (
            <InlineError>{describeApiError(clearCache.error)}</InlineError>
          ) : null}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={clearCache.isPending}
              onClick={() => setConfirmOpen(false)}
            >
              取消
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={clearCache.isPending}
              onClick={clearUnusedCache}
            >
              {clearCache.isPending ? (
                <Spinner data-icon="inline-start" />
              ) : (
                <Trash2Icon data-icon="inline-start" />
              )}
              确认清理
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </SettingsSection>
  )
}
