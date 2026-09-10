import type { LibraryMovie } from '@/api/library'
import { MovieCard } from '@/components/movie'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useUIStore } from '@/stores/ui'

export function LibraryMovieCard({ movie }: { movie: LibraryMovie }) {
  const openPlayer = useUIStore(state => state.openPlayer)
  return (
    <Button
      variant="ghost"
      className="block h-auto min-w-0 cursor-pointer rounded-2xl p-0 text-left whitespace-normal hover:bg-transparent hover:text-current dark:hover:bg-transparent"
      onClick={() => openPlayer(movie.id)}
    >
      <MovieCard movie={movie}>
        {movie.tags.length > 0 ? (
          <Badge variant="secondary" className="max-w-full min-w-0">
            <span className="truncate">{movie.tags[0].name}</span>
            {movie.tags.length > 1 ? (
              <span className="shrink-0">+{movie.tags.length - 1}</span>
            ) : null}
          </Badge>
        ) : null}
        {movie.scrape_status === 'done' ? (
          <Badge variant="success">已刮削</Badge>
        ) : movie.scrape_status === 'failed' ? (
          <Badge variant="destructive" className="[--destructive:oklch(0.577_0.245_27.325)]">
            刮削失败
          </Badge>
        ) : (
          <Badge variant="outline">待刮削</Badge>
        )}
        {movie.watched ? (
          <Badge variant="outline">已观看</Badge>
        ) : (
          <Badge
            variant="secondary"
            className="bg-violet-500/10 text-violet-500 dark:bg-violet-500/20"
          >
            未观看
          </Badge>
        )}
      </MovieCard>
    </Button>
  )
}
