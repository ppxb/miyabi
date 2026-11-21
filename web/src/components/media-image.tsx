import { ImageIcon } from 'lucide-react'
import { useState } from 'react'

import { imageURL } from '@/api/client'
import { cn } from 'cn'
import { useSettingsStore } from '@/stores/settings'

type MediaImageProps = {
  source: string
  original?: string
  loading?: 'eager' | 'lazy'
  blurredBackground?: boolean
  onImageLoad?: (image: HTMLImageElement) => void
  className?: string
}

export function MediaImage(props: MediaImageProps) {
  const nsfwMode = useSettingsStore(state => state.nsfwMode)
  return (
    <MediaImageContent
      key={JSON.stringify([props.source, props.original, nsfwMode])}
      {...props}
      concealed={nsfwMode}
    />
  )
}

function MediaImageContent({
  source,
  original,
  loading = 'lazy',
  blurredBackground = false,
  onImageLoad,
  concealed,
  className
}: MediaImageProps & { concealed: boolean }) {
  const [status, setStatus] = useState<'loading' | 'ready' | 'error'>('loading')
  const showImage = !concealed && source.length > 0 && status !== 'error'
  const ready = !showImage || status === 'ready'

  const imageProps = {
    src: imageURL(source),
    srcSet: original ? `${imageURL(source)} 1x, ${imageURL(original)} 2x` : undefined,
    loading,
    decoding: 'async',
    referrerPolicy: 'no-referrer'
  } as const

  return (
    <div className="relative size-full overflow-hidden bg-muted">
      <ImagePlaceholder />
      {showImage ? (
        <div className={cn('absolute inset-0', !ready && 'opacity-0')}>
          {blurredBackground ? (
            <img
              {...imageProps}
              className="pointer-events-none absolute inset-0 size-full scale-125 object-cover blur-xl brightness-75"
            />
          ) : null}
          <img
            {...imageProps}
            className={cn('relative size-full', className)}
            onLoad={async event => {
              const image = event.currentTarget
              onImageLoad?.(image)
              try {
                await image.decode()
              } catch {
                // onLoad already confirmed the image is displayable; a decode
                // failure must not be treated as a load error.
              }
              setStatus('ready')
            }}
            onError={() => setStatus('error')}
          />
        </div>
      ) : null}
    </div>
  )
}

function ImagePlaceholder() {
  return (
    <div className="flex size-full items-center justify-center bg-muted text-muted-foreground">
      <ImageIcon className="size-6" />
    </div>
  )
}
