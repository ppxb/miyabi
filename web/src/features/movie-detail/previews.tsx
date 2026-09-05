import { useState } from 'react'

import type { PreviewImage } from '@/api/discover'
import { imageURL } from '@/api/client'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

export function MoviePreviews({ code, images }: { code: string; images: PreviewImage[] }) {
  return (
    <section aria-labelledby="movie-previews-title" className="space-y-4">
      <h2 id="movie-previews-title" className="text-xl font-semibold tracking-normal">
        预览图
      </h2>
      {images.length === 0 ? (
        <p className="text-sm text-muted-foreground">暂无预览图</p>
      ) : (
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-4 lg:grid-cols-8">
          {images.map((preview, index) => (
            <PreviewThumbnail
              key={preview.original}
              image={preview}
              alt={`${code} 预览图 ${index + 1}`}
            />
          ))}
        </div>
      )}
    </section>
  )
}

function PreviewThumbnail({ image, alt }: { image: PreviewImage; alt: string }) {
  const [status, setStatus] = useState<'loading' | 'ready' | 'error'>('loading')

  return (
    <div className="relative aspect-video">
      {status === 'loading' ? <Skeleton className="absolute inset-0 size-full rounded-lg" /> : null}
      {status === 'error' ? (
        <div className="absolute inset-0 flex items-center justify-center rounded-lg bg-muted text-xs text-muted-foreground">
          加载失败
        </div>
      ) : null}
      <img
        src={imageURL(image.thumbnail)}
        srcSet={`${imageURL(image.thumbnail)} 1x, ${imageURL(image.original)} 2x`}
        alt={alt}
        loading="lazy"
        decoding="async"
        onLoad={event => {
          void event.currentTarget.decode().then(
            () => setStatus('ready'),
            () => setStatus('error')
          )
        }}
        onError={() => setStatus('error')}
        className={cn(
          'absolute inset-0 size-full rounded-lg object-cover transition-opacity duration-200',
          status === 'ready' ? 'opacity-100' : 'opacity-0'
        )}
      />
    </div>
  )
}
