import { useState, type ReactNode } from 'react'
import { ChevronLeftIcon, ChevronRightIcon, FolderIcon } from 'lucide-react'

import { usePanFiles, type PanDirectory } from '@/api/pan'
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
import { DirectoryBreadcrumbs } from '@/features/settings/directory-breadcrumbs'
import { DirectoryList } from '@/features/settings/directory-list'

export function PanDirectoryPicker({
  accountID,
  selected,
  onSelected,
  children
}: {
  accountID: string
  selected?: PanDirectory
  onSelected: (directory: PanDirectory) => void
  children: ReactNode
}) {
  const [open, setOpen] = useState(false)

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>{children}</DialogTrigger>
      <DialogContent className="grid max-h-[calc(100dvh-2rem)] grid-cols-1 overflow-x-hidden overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>选择媒体目录</DialogTitle>
          <DialogDescription>进入目标文件夹后，点击“选择当前目录”。</DialogDescription>
        </DialogHeader>
        <DirectoryPicker
          accountID={accountID}
          selected={selected}
          onSelected={directory => {
            onSelected(directory)
            setOpen(false)
          }}
        />
      </DialogContent>
    </Dialog>
  )
}

function DirectoryPicker({
  accountID,
  selected,
  onSelected
}: {
  accountID: string
  selected?: PanDirectory
  onSelected: (directory: PanDirectory) => void
}) {
  const [location, setLocation] = useState({ id: selected?.id ?? '0', page: 1 })
  const files = usePanFiles(accountID, location.id, location.page)
  const currentPath = files.data?.path ?? []
  const currentName = currentPath[currentPath.length - 1]?.name ?? '根目录'
  const currentFullPath = currentPath.map(item => item.name).join('/')

  function navigate(id: string, page = 1) {
    setLocation({ id, page })
  }

  return (
    <div className="min-w-0 space-y-4">
      <DirectoryBreadcrumbs
        path={files.data?.path}
        currentID={location.id}
        disabled={files.isFetching}
        onNavigate={id => navigate(id)}
      />

      <DirectoryList files={files} disabled={files.isFetching} onNavigate={id => navigate(id)} />

      <div className="flex items-center justify-between gap-3">
        <span className="text-xs text-muted-foreground">
          第 {location.page} 页{files.data ? ` · 共 ${files.data.total} 项` : ''}
        </span>
        <div className="flex items-center gap-1">
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            disabled={location.page === 1 || files.isFetching}
            onClick={() => navigate(location.id, location.page - 1)}
          >
            <ChevronLeftIcon className="size-4" />
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            disabled={!files.data?.has_more || files.isError || files.isFetching}
            onClick={() => navigate(location.id, location.page + 1)}
          >
            <ChevronRightIcon className="size-4" />
          </Button>
        </div>
      </div>
      {files.isError ? <InlineError>目录读取失败，请稍后重试。</InlineError> : null}
      <div className="flex justify-end">
        <Button
          type="button"
          disabled={!files.data || files.isError || files.isFetching}
          onClick={() =>
            onSelected({
              id: location.id,
              name: currentName,
              path: currentFullPath
            })
          }
        >
          <FolderIcon className="size-4" />
          选择当前目录
        </Button>
      </div>
    </div>
  )
}
