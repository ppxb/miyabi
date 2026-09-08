import {
  ChevronLeftIcon,
  ChevronRightIcon,
  FileIcon,
  FolderIcon,
  LoaderCircleIcon,
  RefreshCwIcon
} from 'lucide-react'
import { Fragment, useState, type ReactNode } from 'react'

import { usePanFiles, useSelectPanDirectory, type PanDirectory } from '@/api/pan'
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator
} from '@/components/ui/breadcrumb'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger
} from '@/components/ui/dialog'

export function PanDirectoryDialog({
  accountID,
  directory,
  children
}: {
  accountID: string
  directory?: PanDirectory
  children: ReactNode
}) {
  const [open, setOpen] = useState(false)

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>{children}</DialogTrigger>
      <DialogContent className="max-h-[calc(100dvh-2rem)] grid-cols-1 overflow-x-hidden overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>选择媒体目录</DialogTitle>
          <DialogDescription>进入目标文件夹后，点击“挂载当前目录”。</DialogDescription>
        </DialogHeader>
        {/* Keep the picker mounted through DialogContent's exit animation. */}
        <DirectoryPicker
          accountID={accountID}
          initialID={directory?.id ?? '0'}
          onSelected={() => setOpen(false)}
        />
      </DialogContent>
    </Dialog>
  )
}

function DirectoryPicker({
  accountID,
  initialID,
  onSelected
}: {
  accountID: string
  initialID: string
  onSelected: () => void
}) {
  const [location, setLocation] = useState({ id: initialID, page: 1 })
  const files = usePanFiles(accountID, location.id, location.page)
  const select = useSelectPanDirectory(accountID)

  function navigate(id: string, page = 1) {
    select.reset()
    setLocation({ id, page })
  }

  return (
    <div className="min-w-0 space-y-4">
      <Breadcrumb aria-label="115 目录路径" className="min-w-0">
        <BreadcrumbList className="gap-1 sm:gap-1">
          <BreadcrumbItem>
            {location.id === '0' ? (
              <BreadcrumbPage>全部文件</BreadcrumbPage>
            ) : (
              <BreadcrumbLink asChild>
                <button type="button" disabled={select.isPending} onClick={() => navigate('0')}>
                  全部文件
                </button>
              </BreadcrumbLink>
            )}
          </BreadcrumbItem>
          {files.data?.path
            .filter(directory => directory.id !== '0')
            .map(directory => (
              <Fragment key={directory.id}>
                <BreadcrumbSeparator className="shrink-0" />
                <BreadcrumbItem className="max-w-full min-w-0">
                  {location.id === directory.id ? (
                    <BreadcrumbPage className="max-w-40 min-w-0 truncate">
                      {directory.name}
                    </BreadcrumbPage>
                  ) : (
                    <BreadcrumbLink asChild>
                      <button
                        type="button"
                        className="max-w-40 min-w-0 truncate"
                        disabled={select.isPending}
                        onClick={() => navigate(directory.id)}
                      >
                        {directory.name}
                      </button>
                    </BreadcrumbLink>
                  )}
                </BreadcrumbItem>
              </Fragment>
            ))}
        </BreadcrumbList>
      </Breadcrumb>
      <div
        className="h-64 min-w-0 overflow-x-hidden overflow-y-auto overscroll-contain rounded-2xl border p-1"
        aria-busy={files.isFetching}
      >
        {files.isPending ? (
          <div
            role="status"
            className="flex h-full items-center justify-center gap-2 text-sm text-muted-foreground"
          >
            <LoaderCircleIcon className="size-4 animate-spin" />
            正在读取目录…
          </div>
        ) : files.isError ? (
          <div className="flex h-full flex-col items-center justify-center gap-3 px-4 text-center">
            <p role="alert" className="text-sm text-muted-foreground">
              目录读取失败，请检查 115 授权和网络后重试。
            </p>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => void files.refetch()}
              disabled={files.isFetching}
            >
              <RefreshCwIcon className="size-4" />
              重试
            </Button>
          </div>
        ) : files.data.files.length === 0 ? (
          <p className="flex h-full items-center justify-center text-sm text-muted-foreground">
            当前目录为空
          </p>
        ) : (
          <ul>
            {files.data.files.map(file => {
              const Icon = file.is_directory ? FolderIcon : FileIcon
              return (
                <li key={file.id}>
                  <Button
                    type="button"
                    variant="ghost"
                    className="w-full max-w-full min-w-0 justify-start rounded-xl"
                    disabled={!file.is_directory || select.isPending}
                    onClick={() => navigate(file.id)}
                  >
                    <Icon className="size-4 shrink-0" />
                    <span className="min-w-0 flex-1 truncate text-left">{file.name}</span>
                    {file.is_directory ? (
                      <ChevronRightIcon className="size-4 shrink-0 text-muted-foreground" />
                    ) : null}
                  </Button>
                </li>
              )
            })}
          </ul>
        )}
      </div>
      <div className="flex items-center justify-between gap-3">
        <span className="text-xs text-muted-foreground">
          第 {location.page} 页{files.data ? ` · 共 ${files.data.total} 项` : ''}
        </span>
        <div className="flex items-center gap-1">
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label="上一页"
            disabled={location.page === 1 || files.isFetching || select.isPending}
            onClick={() => navigate(location.id, location.page - 1)}
          >
            <ChevronLeftIcon className="size-4" />
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label="下一页"
            disabled={
              !files.data?.has_more || files.isError || files.isFetching || select.isPending
            }
            onClick={() => navigate(location.id, location.page + 1)}
          >
            <ChevronRightIcon className="size-4" />
          </Button>
        </div>
      </div>
      {select.isError ? (
        <p role="alert" className="text-sm text-destructive">
          目录挂载未完成，请稍后重试。
        </p>
      ) : null}
      <div className="flex justify-end">
        <Button
          type="button"
          disabled={!files.data || files.isError || files.isFetching || select.isPending}
          onClick={() => select.mutate(location.id, { onSuccess: onSelected })}
        >
          {select.isPending ? (
            <LoaderCircleIcon className="size-4 animate-spin" />
          ) : (
            <FolderIcon className="size-4" />
          )}
          挂载当前目录
        </Button>
      </div>
    </div>
  )
}
