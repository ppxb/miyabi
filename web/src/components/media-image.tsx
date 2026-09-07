import { EyeOffIcon, ImageOffIcon } from 'lucide-react'
import { useState } from 'react'

import { imageURL } from '@/api/client'
import { usePreferences } from '@/api/settings'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

type MediaImageProps = {
  source: string
  original?: string
  alt: string
  loading?: 'eager' | 'lazy'
  className?: string
}

// All media images pass through this component so hidden images have no img/src.
export function MediaImage(props: MediaImageProps) {
  const preferences = usePreferences()
  if (preferences.data?.nsfw_mode !== false) {
    return (
      <div
        role="img"
        aria-label="图片已隐藏"
        className="flex size-full items-center justify-center bg-muted text-muted-foreground"
      >
        <EyeOffIcon className="size-6" aria-hidden="true" />
      </div>
    )
  }
  return <VisibleImage key={props.source} {...props} />
}

function VisibleImage({ source, original, alt, loading = 'lazy', className }: MediaImageProps) {
  const [status, setStatus] = useState<'loading' | 'ready' | 'error'>('loading')

  if (status === 'error') {
    return (
      <div
        role="img"
        aria-label={`${alt}加载失败`}
        className="flex size-full items-center justify-center bg-muted text-muted-foreground"
      >
        <ImageOffIcon className="size-6" aria-hidden="true" />
      </div>
    )
  }

  return (
    <div className="relative size-full">
      {status === 'loading' ? (
        <Skeleton className="absolute inset-0 size-full rounded-none" />
      ) : null}
      <img
        src={imageURL(source)}
        srcSet={original ? `${imageURL(source)} 1x, ${imageURL(original)} 2x` : undefined}
        alt={alt}
        loading={loading}
        decoding="async"
        onLoad={() => setStatus('ready')}
        onError={() => setStatus('error')}
        className={cn(
          'size-full transition-opacity duration-200',
          status === 'ready' ? 'opacity-100' : 'opacity-0',
          className
        )}
      />
    </div>
  )
}
