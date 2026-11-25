import { useIsMovieViewed } from '@/api/browse-history'
import type { MovieIdentity } from '@/api/movie-states'
import { useMovieState } from '@/api/movie-states'
import { Badge } from '@/components/ui/badge'

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
    return <Badge variant="downloading">下载中</Badge>
  }
  if (state === 'processing') {
    return <Badge variant="frosted">入库处理中</Badge>
  }
  if (!hideViewed && isViewed) {
    return <Badge variant="success">已浏览</Badge>
  }
  return null
}
