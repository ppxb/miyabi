import { RotateCcwIcon, ScanLineIcon } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'

export function LibraryScanButton({
  rebuild,
  loading,
  scanning,
  active,
  connected,
  pending,
  blocked,
  onSubmit
}: {
  rebuild: boolean
  loading: boolean
  scanning: boolean
  active: boolean
  connected: boolean
  pending: boolean
  blocked: boolean
  onSubmit: (rebuild: boolean) => void
}) {
  if (loading) return <Skeleton className="h-8 w-28 rounded-lg" />
  const busy = active && connected
  const label = active
    ? connected
      ? rebuild
        ? '正在重建'
        : '正在扫描'
      : '连接中'
    : rebuild
      ? '重建媒体库'
      : '扫描并修复'

  return (
    <Button
      variant={rebuild ? 'outline' : 'default'}
      disabled={scanning || pending || blocked}
      onClick={() => onSubmit(rebuild)}
    >
      {busy || pending ? (
        <Spinner data-icon="inline-start" />
      ) : rebuild ? (
        <RotateCcwIcon data-icon="inline-start" />
      ) : (
        <ScanLineIcon data-icon="inline-start" />
      )}
      {label}
    </Button>
  )
}
