import { ChevronLeftIcon, ChevronRightIcon, FolderIcon, LoaderCircleIcon } from 'lucide-react'
import { useState, type ReactNode } from 'react'

import { usePanFiles, useSelectPanDirectory, type PanDirectory } from '@/api/pan'
import { InlineError } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger
} from '@/components/ui/dialog'
import { DirectoryBreadcrumbs } from './directory-breadcrumbs'
import { DirectoryList } from './directory-list'

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
          <DialogDescription>
            进入目标文件夹后，点击“挂载当前目录”，系统会自动同步媒体库。
          </DialogDescription>
        </DialogHeader>
        {/* Keep the picker mounted through DialogContent's exit animation. */}
        <DirectoryPicker
          accountID={accountID}
          initialID={directory?.id ?? '0'}
          mountedID={directory?.id}
          onSelected={() => setOpen(false)}
        />
      </DialogContent>
    </Dialog>
  )
}

function DirectoryPicker({
  accountID,
  initialID,
  mountedID,
  onSelected
}: {
  accountID: string
  initialID: string
  mountedID?: string
  onSelected: () => void
}) {
  const [location, setLocation] = useState({ id: initialID, page: 1 })
  const files = usePanFiles(accountID, location.id, location.page)
  const select = useSelectPanDirectory(accountID)
  const mounted = location.id === mountedID

  function navigate(id: string, page = 1) {
    select.reset()
    setLocation({ id, page })
  }

  return (
    <div className="min-w-0 space-y-4">
      <DirectoryBreadcrumbs
        path={files.data?.path}
        currentID={location.id}
        disabled={select.isPending}
        onNavigate={id => navigate(id)}
      />

      <DirectoryList files={files} disabled={select.isPending} onNavigate={id => navigate(id)} />

      <div className="flex items-center justify-between gap-3">
        <span className="text-xs text-muted-foreground">
          第 {location.page} 页{files.data ? ` · 共 ${files.data.total} 项` : ''}
        </span>
        <div className="flex items-center gap-1">
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            disabled={location.page === 1 || files.isFetching || select.isPending}
            onClick={() => navigate(location.id, location.page - 1)}
          >
            <ChevronLeftIcon className="size-4" />
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            disabled={
              !files.data?.has_more || files.isError || files.isFetching || select.isPending
            }
            onClick={() => navigate(location.id, location.page + 1)}
          >
            <ChevronRightIcon className="size-4" />
          </Button>
        </div>
      </div>
      {select.isError ? <InlineError>目录挂载未完成，请稍后重试。</InlineError> : null}
      <div className="flex justify-end">
        <Button
          type="button"
          disabled={mounted || !files.data || files.isError || files.isFetching || select.isPending}
          onClick={() => select.mutate(location.id, { onSuccess: onSelected })}
        >
          {select.isPending ? (
            <LoaderCircleIcon className="size-4 animate-spin" />
          ) : (
            <FolderIcon className="size-4" />
          )}
          {mounted ? '当前已挂载' : '挂载当前目录'}
        </Button>
      </div>
    </div>
  )
}
