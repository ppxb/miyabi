import { Link } from '@tanstack/react-router'
import { LoaderCircleIcon } from 'lucide-react'

import { ApiError } from '@/api/client'
import { Button } from '@/components/ui/button'
import { useUIStore } from '@/stores/ui'

export function PlayerLoading() {
  return (
    <div className="grid aspect-video max-h-[65dvh] place-items-center rounded-xl bg-muted">
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <LoaderCircleIcon className="size-5 animate-spin" />
        正在准备播放…
      </div>
    </div>
  )
}

export function PlayerError({
  error,
  message = '加载失败，请检查服务后重试。',
  onRetry
}: {
  error?: Error
  message?: string
  onRetry: () => void
}) {
  const close = useUIStore(state => state.closePlayer)
  const status = error instanceof ApiError ? error.status : undefined
  const needsSettings = status === 401 || status === 400
  const description =
    status === 401
      ? '请先在设置中登录 115，然后重新播放。'
      : status === 400
        ? '请先在设置中挂载媒体目录。'
        : status === 404
          ? '当前媒体目录中没有对应文件，请重新扫描媒体库。'
          : error instanceof ApiError
            ? error.message
            : message

  return (
    <div className="grid aspect-video max-h-[65dvh] place-items-center rounded-xl bg-muted p-5">
      <div className="max-w-md space-y-4 text-center">
        <p className="text-sm text-muted-foreground">{description}</p>
        <div className="flex flex-wrap justify-center gap-2">
          {needsSettings ? (
            <Button asChild size="sm">
              <Link to="/settings" onClick={close}>
                前往设置
              </Link>
            </Button>
          ) : null}
          <Button variant="outline" size="sm" onClick={onRetry}>
            重新加载
          </Button>
        </div>
      </div>
    </div>
  )
}
