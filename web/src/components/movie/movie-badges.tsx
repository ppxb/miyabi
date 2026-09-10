import type { DiscoverMovie } from '@/api/discover'
import { useMovieState } from '@/api/movie-states'
import { Badge } from '@/components/ui/badge'

export function MovieResourceBadges({ movie }: { movie: DiscoverMovie }) {
  return (
    <>
      {movie.has_subtitle ? <Badge variant="outline">字幕</Badge> : null}
      {movie.has_preview ? <Badge variant="outline">预览</Badge> : null}
      {movie.magnets_count > 0 ? <Badge variant="outline">含磁力</Badge> : null}
      {movie.release_status === 'upcoming' ? <Badge variant="outline">即将发行</Badge> : null}
    </>
  )
}

export function MovieStateBadge({ movie }: { movie: DiscoverMovie }) {
  const { state } = useMovieState(movie)
  if (state === 'in_library') {
    return (
      <Badge className="border-emerald-200 bg-emerald-100 text-emerald-800 dark:border-emerald-800 dark:bg-emerald-950 dark:text-emerald-300">
        已入库
      </Badge>
    )
  }
  if (state === 'saving') {
    return <Badge variant="outline">下载中</Badge>
  }
  if (state === 'processing') {
    return <Badge variant="outline">入库处理中</Badge>
  }
  return null
}
