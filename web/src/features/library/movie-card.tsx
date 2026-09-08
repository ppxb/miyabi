import { FilmIcon } from 'lucide-react'

import type { LibraryMovie } from '@/api/library'
import { MediaImage } from '@/components/media-image'
import { OverflowTooltip } from '@/components/overflow-tooltip'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { LibraryFilesDialog } from '@/features/library/files-dialog'
import { formatSize } from '@/lib/format'

const scrapeLabels = { pending: '待刮削', done: '已刮削', failed: '刮削失败' }

export function LibraryMovieCard({ movie }: { movie: LibraryMovie }) {
  return (
    <LibraryFilesDialog movie={movie}>
      <Button
        variant="ghost"
        className="block h-auto min-w-0 rounded-2xl p-0 text-left whitespace-normal"
        aria-label={`查看 ${movie.code} 的文件`}
      >
        <Card
          size="sm"
          className="h-full gap-0 overflow-hidden py-0 transition-shadow hover:shadow-xl"
        >
          <div className="relative flex aspect-3/2 items-center justify-center bg-muted">
            {movie.cover ? (
              <div className="absolute inset-0">
                <MediaImage source={movie.cover} className="object-cover" />
              </div>
            ) : (
              <FilmIcon className="size-8 text-muted-foreground/60" />
            )}
            <Badge
              variant="outline"
              className="absolute top-2 left-2 max-w-[calc(100%-1rem)] truncate bg-background/85"
            >
              {movie.code}
            </Badge>
          </div>
          <CardContent className="min-w-0 space-y-2 p-3">
            <OverflowTooltip content={movie.title || movie.code}>
              <h3 className="truncate text-sm leading-5 font-semibold">
                {movie.title || movie.code}
              </h3>
            </OverflowTooltip>
            <p className="text-xs text-muted-foreground">
              {movie.file_count} 个视频 · {formatSize(movie.size)}
            </p>
            <Badge variant={movie.scrape_status === 'failed' ? 'destructive' : 'secondary'}>
              {scrapeLabels[movie.scrape_status]}
            </Badge>
          </CardContent>
        </Card>
      </Button>
    </LibraryFilesDialog>
  )
}
