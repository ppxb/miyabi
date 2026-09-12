import type { LibraryMovie } from '@/api/library'
import { Badge } from '@/components/ui/badge'

export function LibraryMovieStatus({ movie }: { movie: LibraryMovie }) {
  return (
    <>
      {movie.scrape_status === 'done' ? (
        <Badge variant="success">已刮削</Badge>
      ) : movie.scrape_status === 'failed' ? (
        <Badge variant="destructive" className="[--destructive:oklch(0.577_0.245_27.325)]">
          刮削失败
        </Badge>
      ) : (
        <Badge variant="outline">待刮削</Badge>
      )}
      {!movie.watched ? (
        <Badge variant="secondary" className="bg-violet-500 text-white">
          新入库
        </Badge>
      ) : null}
    </>
  )
}
