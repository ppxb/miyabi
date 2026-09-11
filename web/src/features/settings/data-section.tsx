import { DatabaseIcon, LoaderCircleIcon, RefreshCwIcon, Trash2Icon } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'

import { useClearCache, useDataInfo } from '@/api/data'
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
import { formatSize } from '@/lib/format'
import { cn } from '@/lib/utils'
import { SettingRow, SettingsSection } from './shared'

const numberFormat = new Intl.NumberFormat('zh-CN')

export function DataSection() {
  const info = useDataInfo()
  const clearCache = useClearCache()
  const [confirmOpen, setConfirmOpen] = useState(false)
  const cache = info.data?.cache
  const busy = info.isFetching || clearCache.isPending

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
        description={
          info.isPending ? (
            <Skeleton className="h-4 w-56 max-w-full" />
          ) : (
            (info.data?.data_directory ?? '无法读取数据目录')
          )
        }
      >
        <Button
          type="button"
          variant="outline"
          size="icon"
          aria-label="刷新数据与缓存统计"
          title="刷新统计"
          disabled={busy}
          onClick={() => void info.refetch()}
        >
          <RefreshCwIcon className={cn('size-4', info.isFetching && 'animate-spin')} />
        </Button>
      </SettingRow>
      <SettingRow title="数据库" description="媒体库索引、观看记录和应用设置，包含当前日志文件">
        {info.isPending ? (
          <Skeleton className="h-5 w-24" />
        ) : (
          <span className="text-sm font-medium tabular-nums">
            {info.data ? formatSize(info.data.database_size_bytes) : '—'}
          </span>
        )}
      </SettingRow>
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
          size="sm"
          disabled={busy || info.isError || !cache || cache.unused_entry_count === 0}
          onClick={openConfirmation}
        >
          {clearCache.isPending ? <LoaderCircleIcon className="animate-spin" /> : <Trash2Icon />}
          清理所有缓存
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
              仅删除未被影片或未完成任务引用的图片。媒体库封面、观看记录和 115 网盘文件会保留。
            </DialogDescription>
          </DialogHeader>
          {clearCache.error ? <InlineError>{clearCache.error.message}</InlineError> : null}
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
                <LoaderCircleIcon className="animate-spin" />
              ) : (
                <Trash2Icon />
              )}
              确认清理
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </SettingsSection>
  )
}
