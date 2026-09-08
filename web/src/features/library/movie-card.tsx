import type { LibraryMovie } from '@/api/library'
import { MovieCard } from '@/components/movie'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { LibraryFilesDialog } from '@/features/library/files-dialog'
import { formatSize } from '@/lib/format'

const scrapeLabels = { pending: '待刮削', done: '已刮削', failed: '刮削失败' }

export function LibraryMovieCard({ movie }: { movie: LibraryMovie }) {
  return (
    <LibraryFilesDialog movie={movie}>
      <Button
        variant="ghost"
        className="block h-auto min-w-0 rounded-2xl p-0 text-left whitespace-normal"
      >
        <MovieCard
          movie={movie}
          description={`${movie.file_count} 个视频 · ${formatSize(movie.size)}`}
        >
          <Badge variant={movie.scrape_status === 'failed' ? 'destructive' : 'secondary'}>
            {scrapeLabels[movie.scrape_status]}
          </Badge>
        </MovieCard>
      </Button>
    </LibraryFilesDialog>
  )
}
