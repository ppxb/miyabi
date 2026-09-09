import type { LibraryMovie } from '@/api/library'
import { MovieCard } from '@/components/movie'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { formatSize } from '@/lib/format'
import { useUIStore } from '@/stores/ui'

const scrapeLabels = { pending: '待刮削', done: '已刮削', failed: '刮削失败' }

export function LibraryMovieCard({ movie }: { movie: LibraryMovie }) {
  const openPlayer = useUIStore(state => state.openPlayer)
  return (
    <Button
      variant="ghost"
      className="block h-auto min-w-0 cursor-pointer rounded-2xl p-0 text-left whitespace-normal hover:bg-transparent hover:text-current dark:hover:bg-transparent"
      onClick={() => openPlayer(movie.id)}
    >
      <MovieCard
        movie={movie}
        description={`${movie.file_count} 个视频 • ${formatSize(movie.size)}`}
      >
        <Badge variant={movie.scrape_status === 'failed' ? 'destructive' : 'secondary'}>
          {scrapeLabels[movie.scrape_status]}
        </Badge>
      </MovieCard>
    </Button>
  )
}
