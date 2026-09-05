import { ImageOffIcon } from 'lucide-react'
import { useState } from 'react'

import { imageURL } from '@/api/client'

export function MovieCover({
  source,
  alt,
  loading = 'lazy'
}: {
  source: string
  alt: string
  loading?: 'eager' | 'lazy'
}) {
  const [failed, setFailed] = useState(false)

  if (failed) {
    return (
      <div
        role="img"
        aria-label={`${alt}加载失败`}
        className="flex size-full flex-col items-center justify-center gap-2 text-sm text-muted-foreground"
      >
        <ImageOffIcon className="size-6" aria-hidden="true" />
        <span>封面加载失败</span>
      </div>
    )
  }

  return (
    <img
      src={imageURL(source)}
      alt={alt}
      loading={loading}
      decoding="async"
      onError={() => setFailed(true)}
      className="h-full w-full object-contain"
    />
  )
}
