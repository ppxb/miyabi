import { ImageIcon } from 'lucide-react'
import { useState } from 'react'

import { imageURL } from '@/api/client'
import { cn } from '@/lib/utils'
import { useSettingsStore } from '@/stores/settings'

type MediaImageProps = {
  source: string
  original?: string
  loading?: 'eager' | 'lazy'
  blurredBackground?: boolean
  className?: string
}

export function MediaImage(props: MediaImageProps) {
  return <MediaImageContent key={JSON.stringify([props.source, props.original])} {...props} />
}

function MediaImageContent({
  source,
  original,
  loading = 'lazy',
  blurredBackground = false,
  className
}: MediaImageProps) {
  const [hasImageError, setHasImageError] = useState(false)
  const nsfwMode = useSettingsStore(state => state.nsfwMode)
  const shouldShowImage = !nsfwMode && source.length > 0 && !hasImageError
  const imageProps = {
    src: imageURL(source),
    srcSet: original ? `${imageURL(source)} 1x, ${imageURL(original)} 2x` : undefined,
    loading,
    decoding: 'async',
    referrerPolicy: 'no-referrer',
    onError: () => setHasImageError(true)
  } as const

  return (
    <div className="relative size-full overflow-hidden bg-muted">
      {shouldShowImage ? (
        <>
          {blurredBackground ? (
            <img
              {...imageProps}
              className="pointer-events-none absolute inset-0 size-full scale-125 object-cover blur-xl brightness-75"
            />
          ) : null}
          <img {...imageProps} className={cn('relative size-full', className)} />
        </>
      ) : (
        <div className="flex size-full items-center justify-center text-muted-foreground">
          <ImageIcon className="size-6" />
        </div>
      )}
    </div>
  )
}
