import { useIsMovieViewed } from '@/api/browse-history'
import type { DiscoverMovie } from '@/api/discover'
import type { MovieIdentity } from '@/api/movie-states'
import { useMovieState } from '@/api/movie-states'
import { Badge } from '@/components/ui/badge'

export function MovieResourceBadges({ movie }: { movie: DiscoverMovie }) {
  return (
    <>
      {movie.has_subtitle ? <Badge variant="outline">字幕</Badge> : null}
      {movie.has_preview ? <Badge variant="outline">有预览</Badge> : null}
      {movie.magnets_count > 0 ? <Badge variant="outline">含磁力</Badge> : null}
      {movie.release_status === 'upcoming' ? <Badge variant="outline">即将发行</Badge> : null}
    </>
  )
}

export function MovieStateBadge({
  movie,
  hideViewed = false
}: {
  movie: MovieIdentity
  hideViewed?: boolean
}) {
  const { state } = useMovieState(movie)
  const isViewed = useIsMovieViewed(movie.id)

  if (state === 'in_library') {
    return <Badge variant="library">已入库</Badge>
  }
  if (state === 'saving') {
    return <Badge variant="frosted">下载中</Badge>
  }
  if (state === 'processing') {
    return <Badge variant="frosted">入库处理中</Badge>
  }
  if (!hideViewed && isViewed) {
    return <Badge variant="success">已浏览</Badge>
  }
  return null
}
