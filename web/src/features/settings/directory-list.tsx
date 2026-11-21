import { ChevronRightIcon, FileIcon, FolderIcon } from 'lucide-react'

import type { usePanAccountFiles } from '@/api/pan'
import { InlineError } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'

type DirectoryListProps = {
  files: ReturnType<typeof usePanAccountFiles>
  disabled?: boolean
  onNavigate: (id: string) => void
}

export function DirectoryList({ files, disabled = false, onNavigate }: DirectoryListProps) {
  return (
    <div className="h-64 min-w-0 overflow-x-hidden overflow-y-auto overscroll-contain rounded-2xl border p-1">
      {files.isPending ? (
        <div className="flex h-full items-center justify-center gap-2 text-sm text-muted-foreground">
          <Spinner />
          正在读取目录…
        </div>
      ) : files.isError ? (
        <InlineError
          className="h-full flex-col justify-center px-4 text-center"
          onRetry={() => void files.refetch()}
          retrying={files.isFetching}
        >
          目录读取失败，请检查 115 授权和网络后重试。
        </InlineError>
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
                  disabled={!file.is_directory || disabled}
                  onClick={() => onNavigate(file.id)}
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
  )
}
