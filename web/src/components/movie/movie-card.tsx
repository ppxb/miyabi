import { ImageIcon } from 'lucide-react'
import { useState } from 'react'

import type { DiscoverMovie } from '@/api/discover'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'

export function MovieCard({ movie }: { movie: DiscoverMovie }) {
  const image = movie.thumbnail || movie.cover
  const [failedImage, setFailedImage] = useState('')

  return (
    <Card size="sm" className="group gap-0 overflow-hidden py-0 transition-shadow hover:shadow-xl">
      <div className="relative aspect-3/2 overflow-hidden bg-muted">
        {image && failedImage !== image ? (
          <img
            src={image}
            alt=""
            loading="lazy"
            decoding="async"
            referrerPolicy="no-referrer"
            className="h-full w-full object-cover transition-transform duration-300 group-hover:scale-[1.02]"
            onError={() => setFailedImage(image)}
          />
        ) : (
          <div className="flex h-full items-center justify-center text-muted-foreground">
            <ImageIcon className="size-6" />
          </div>
        )}

        <div className="absolute top-2 left-2 flex max-w-[calc(100%-1rem)] flex-wrap gap-1.5">
          <Badge className="border-white/15 bg-black/55 text-white backdrop-blur">
            {movie.code}
          </Badge>
          <MovieStateBadge movie={movie} />
        </div>
      </div>

      <CardContent className="space-y-2 p-3">
        <h3 className="line-clamp-2 min-h-10 text-sm leading-5 font-semibold" title={movie.title}>
          {movie.title || movie.origin_title || movie.code}
        </h3>
        <div className="flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
          <span>{movie.release_date || '日期未知'}</span>
          {movie.duration > 0 ? <span>{movie.duration} 分钟</span> : null}
          {movie.rating > 0 ? <span className="ml-auto">{movie.rating.toFixed(1)}</span> : null}
        </div>
        <div className="flex flex-wrap gap-1.5">
          {movie.has_subtitle ? <Badge variant="secondary">字幕</Badge> : null}
          {movie.has_preview ? <Badge variant="secondary">预览</Badge> : null}
          <Badge variant={movie.magnets_count > 0 ? 'outline' : 'ghost'}>
            {movie.magnets_count > 0 ? `${movie.magnets_count} 个磁力` : '暂无磁力'}
          </Badge>
        </div>
      </CardContent>
    </Card>
  )
}

function MovieStateBadge({ movie }: { movie: DiscoverMovie }) {
  if (movie.state === 'in_library') {
    return <Badge className="bg-emerald-600 text-white">已入库</Badge>
  }
  if (movie.state === 'saving') {
    return <Badge className="bg-amber-500 text-white">保存中</Badge>
  }
  if (movie.release_status === 'upcoming') {
    return <Badge variant="secondary">即将发行</Badge>
  }
  return null
}
