import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { LoaderCircleIcon, XIcon } from 'lucide-react'

import { ApiError } from '@/api/client'
import { Button } from '@/components/ui/button'
import { useUIStore } from '@/stores/ui'

export function PlayerCloseButton() {
  const close = useUIStore(state => state.closePlayer)

  return (
    <Button
      variant="ghost"
      size="icon-sm"
      className="cursor-pointer rounded-full bg-white/10 text-white backdrop-blur-xl hover:bg-white/20 hover:text-white dark:hover:bg-white/20"
      onClick={close}
    >
      <XIcon />
    </Button>
  )
}

type PlayerHeaderProps = {
  title?: string
  toolbar?: ReactNode
}

export function PlayerTitle({ title }: { title?: string }) {
  return <p className="min-w-0 flex-1 truncate text-sm font-medium text-white/90">{title}</p>
}

function PlayerStatus({ children, title, toolbar }: PlayerHeaderProps & { children: ReactNode }) {
  return (
    <div className="relative grid aspect-video size-full place-items-center bg-black px-6 pt-16 pb-6 text-white">
      <div className="absolute inset-x-0 top-0 flex min-w-0 items-center gap-2 p-3 sm:p-4">
        <PlayerTitle title={title} />
        {toolbar}
        <PlayerCloseButton />
      </div>
      {children}
    </div>
  )
}

export function PlayerLoading({ title, toolbar }: PlayerHeaderProps) {
  return (
    <PlayerStatus title={title} toolbar={toolbar}>
      <div className="flex items-center gap-2 text-sm text-white/60">
        <LoaderCircleIcon className="size-5 animate-spin" />
        正在准备播放…
      </div>
    </PlayerStatus>
  )
}

export function PlayerError({
  error,
  message = '加载失败，请检查服务后重试。',
  onRetry,
  title,
  toolbar
}: PlayerHeaderProps & {
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
    <PlayerStatus title={title} toolbar={toolbar}>
      <div className="max-w-md space-y-4 text-center">
        <p className="text-sm text-white/60">{description}</p>
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
    </PlayerStatus>
  )
}
