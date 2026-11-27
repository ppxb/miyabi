import { ImageIcon } from 'lucide-react'
import { useState } from 'react'

import { imageURL } from '@/api/client'
import { cn } from 'cn'
import { useSettingsStore } from '@/stores/settings'

type MediaImageProps = {
  source: string
  alt?: string
  original?: string
  loading?: 'eager' | 'lazy'
  blurredBackground?: boolean
  /** Requested thumbnail width in pixels; the proxy skips images that are already narrower. */
  width?: number
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
  alt,
  original,
  loading = 'lazy',
  blurredBackground = false,
  width,
  onImageLoad,
  concealed,
  className
}: MediaImageProps & { concealed: boolean }) {
  const [status, setStatus] = useState<'loading' | 'ready' | 'error'>('loading')
  const showImage = !concealed && source.length > 0 && status !== 'error'
  const ready = !showImage || status === 'ready'

  const imageProps = {
    src: imageURL(source, width),
    srcSet: original
      ? `${imageURL(source, width)} 1x, ${imageURL(original, width ? width * 2 : undefined)} 2x`
      : undefined,
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
            <div
              className="pointer-events-none absolute inset-0 scale-125 bg-cover bg-center blur-xl brightness-75"
              style={{ backgroundImage: `url("${imageURL(source, width)}")` }}
            />
          ) : null}
          <img
            {...imageProps}
            alt={alt}
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
