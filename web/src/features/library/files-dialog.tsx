import { FileVideoIcon } from 'lucide-react'
import { useState, type ReactNode } from 'react'

import { useLibraryFiles } from '@/api/library'
import { ListPagination } from '@/components/list-pagination'
import { OverflowTooltip } from '@/components/overflow-tooltip'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger
} from '@/components/ui/dialog'
import { formatSize } from '@/lib/format'

export function LibraryFilesDialog({ children }: { children: ReactNode }) {
  return (
    <Dialog>
      <DialogTrigger asChild>{children}</DialogTrigger>
      <DialogContent className="max-h-[85dvh] min-w-0 overflow-hidden sm:max-w-2xl">
        <DialogHeader className="min-w-0 pr-6">
          <DialogTitle>未识别的视频</DialogTitle>
          <DialogDescription>
            未从文件名识别出番号。整理 115 中的文件名后可重新扫描。
          </DialogDescription>
        </DialogHeader>
        <LibraryFilesList />
      </DialogContent>
    </Dialog>
  )
}

function LibraryFilesList() {
  const [page, setPage] = useState(1)
  const files = useLibraryFiles(undefined, true, page)

  if (files.isPending || files.isPlaceholderData) {
    return (
      <div className="max-h-[50dvh] divide-y divide-border overflow-hidden">
        {Array.from({ length: 6 }, (_, index) => (
          <div key={index} className="flex items-center gap-3 py-3">
            <Skeleton className="size-5 shrink-0" />
            <div className="flex-1 space-y-2">
              <Skeleton className="h-4 w-3/4" />
              <Skeleton className="h-3 w-1/2" />
            </div>
            <Skeleton className="h-3 w-12" />
          </div>
        ))}
      </div>
    )
  }
  if (files.isError) {
    return (
      <div className="space-y-3 text-center">
        <p className="text-sm text-muted-foreground">无法读取文件索引，请检查后端服务后重试。</p>
        <Button variant="outline" onClick={() => void files.refetch()}>
          重试
        </Button>
      </div>
    )
  }

  return (
    <div className="min-h-0 min-w-0 space-y-2">
      <div className="max-h-[50dvh] min-w-0 overflow-x-hidden overflow-y-auto">
        <ul className="min-w-0 divide-y divide-border">
          {files.data.files.map(file => (
            <li key={file.id} className="flex min-w-0 items-center gap-3 py-3">
              <FileVideoIcon className="size-5 shrink-0 text-muted-foreground" />
              <div className="min-w-0 flex-1 space-y-1">
                <OverflowTooltip content={file.name}>
                  <p className="truncate text-sm font-medium">{file.name}</p>
                </OverflowTooltip>
                <OverflowTooltip content={file.path}>
                  <p className="truncate text-xs text-muted-foreground">{file.path}</p>
                </OverflowTooltip>
              </div>
              <span className="shrink-0 text-xs text-muted-foreground tabular-nums">
                {formatSize(file.size)}
              </span>
            </li>
          ))}
        </ul>
        {files.data.files.length === 0 ? (
          <p className="py-8 text-center text-sm text-muted-foreground">没有对应的文件索引。</p>
        ) : null}
      </div>
      {page > 1 || files.data.has_more ? (
        <ListPagination
          page={page}
          hasMore={files.data.has_more}
          disabled={files.isFetching}
          onPageChange={setPage}
          scrollToTop={false}
        />
      ) : null}
    </div>
  )
}
