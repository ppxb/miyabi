import type { DiscoverMovie, MovieState } from '@/api/discover'
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

export function MovieStateBadge({ state }: { state: MovieState }) {
  if (state === 'in_library') {
    return <Badge variant="outline">已入库</Badge>
  }
  if (state === 'saving') {
    return <Badge variant="outline">保存中</Badge>
  }
  return null
}
