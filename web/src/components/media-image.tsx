import { ImageIcon } from 'lucide-react'
import { useState } from 'react'

import { imageURL } from '@/api/client'
import { cn } from '@/lib/utils'
import { useSettingsStore } from '@/stores/settings'

type MediaImageProps = {
  source: string
  original?: string
  loading?: 'eager' | 'lazy'
  className?: string
}

export function MediaImage(props: MediaImageProps) {
  return <MediaImageContent key={JSON.stringify([props.source, props.original])} {...props} />
}

function MediaImageContent({ source, original, loading = 'lazy', className }: MediaImageProps) {
  const [hasImageError, setHasImageError] = useState(false)
  const nsfwMode = useSettingsStore(state => state.nsfwMode)
  const shouldShowImage = !nsfwMode && source.length > 0 && !hasImageError

  return (
    <div className="relative size-full overflow-hidden bg-muted">
      {shouldShowImage ? (
        <img
          src={imageURL(source)}
          srcSet={original ? `${imageURL(source)} 1x, ${imageURL(original)} 2x` : undefined}
          loading={loading}
          decoding="async"
          referrerPolicy="no-referrer"
          onError={() => setHasImageError(true)}
          className={cn('size-full', className)}
        />
      ) : (
        <div className="flex size-full items-center justify-center text-muted-foreground">
          <ImageIcon className="size-6" />
        </div>
      )}
    </div>
  )
}
