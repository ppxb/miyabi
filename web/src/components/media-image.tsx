import { useRouter } from '@tanstack/react-router'
import { ImageIcon } from 'lucide-react'
import { useEffect, useState } from 'react'

import { imageURL } from '@/api/client'
import { cn } from 'cn'
import { useSettingsStore, type ImageSafety } from '@/stores/settings'

export type MediaImageProps = {
  source: string
  original?: string
  label?: string
  description?: string
  compact?: boolean
  loading?: 'eager' | 'lazy'
  blurredBackground?: boolean
  onReady?: () => void
  onImageLoad?: (image: HTMLImageElement) => void
  className?: string
}

export function MediaImage(props: MediaImageProps) {
  const imageSafety = useSettingsStore(state => state.imageSafety)
  return (
    <MediaImageContent
      key={JSON.stringify([props.source, props.original, imageSafety])}
      {...props}
      imageSafety={imageSafety}
    />
  )
}

function MediaImageContent({
  source,
  original,
  label = '图片',
  description,
  compact = false,
  loading = 'lazy',
  blurredBackground = false,
  onReady,
  onImageLoad,
  imageSafety,
  className
}: MediaImageProps & { imageSafety: ImageSafety }) {
  const router = useRouter({ warn: false })
  const [status, setStatus] = useState<'loading' | 'ready' | 'error'>('loading')
  const [revealed, setRevealed] = useState(false)
  const showImage = imageSafety !== 'hide' && source.length > 0 && status !== 'error'
  const blurred = imageSafety === 'blur' && !revealed
  const ready = !showImage || status === 'ready'

  useEffect(() => {
    if (ready) onReady?.()
  }, [ready, onReady])

  useEffect(() => {
    if (!revealed) return
    const conceal = () => setRevealed(false)
    const unsubscribe = router?.subscribe('onBeforeNavigate', conceal)
    window.addEventListener('blur', conceal)
    document.addEventListener('visibilitychange', conceal)
    return () => {
      unsubscribe?.()
      window.removeEventListener('blur', conceal)
      document.removeEventListener('visibilitychange', conceal)
    }
  }, [revealed, router])

  const imageProps = {
    src: imageURL(source),
    srcSet: original ? `${imageURL(source)} 1x, ${imageURL(original)} 2x` : undefined,
    alt: '',
    loading,
    decoding: 'async',
    referrerPolicy: 'no-referrer'
  } as const

  return (
    <div
      className="relative size-full overflow-hidden bg-muted"
      onMouseLeave={() => setRevealed(false)}
      onBlur={event => {
        if (!event.currentTarget.contains(event.relatedTarget)) setRevealed(false)
      }}
    >
      <ImagePlaceholder label={label} description={description} compact={compact} />
      {showImage ? (
        <div className={cn('absolute inset-0', !ready && 'opacity-0')}>
          <div
            className={cn(
              'size-full transition-[filter,transform] duration-200 motion-reduce:transition-none',
              blurred && 'scale-125 blur-[32px]'
            )}
          >
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
                  if (typeof image.decode === 'function') await image.decode()
                  setStatus('ready')
                } catch {
                  setStatus('error')
                }
              }}
              onError={() => setStatus('error')}
            />
          </div>
        </div>
      ) : null}
      {showImage && imageSafety === 'blur' ? (
        <button
          type="button"
          aria-label={`${revealed ? '重新模糊' : '临时显示'} ${label}`}
          aria-pressed={revealed}
          className="absolute inset-0 flex cursor-pointer items-end justify-center p-2 outline-offset-[-3px] focus-visible:outline-2 focus-visible:outline-ring"
          onClick={event => {
            event.preventDefault()
            event.stopPropagation()
            setRevealed(current => !current)
          }}
          onKeyDown={event => event.stopPropagation()}
        >
          <span className="rounded-md bg-black/60 px-2 py-1 text-xs text-white">
            {compact
              ? revealed
                ? '模糊'
                : '显示'
              : revealed
                ? '离开或失焦后恢复模糊'
                : '点击或点按临时显示'}
          </span>
        </button>
      ) : null}
    </div>
  )
}

function ImagePlaceholder({
  label,
  description,
  compact
}: {
  label: string
  description?: string
  compact: boolean
}) {
  return (
    <div
      className={cn(
        'flex size-full flex-col items-center justify-center gap-2 bg-muted text-center text-muted-foreground',
        compact ? 'p-1' : 'px-3 py-6'
      )}
    >
      {!compact ? <ImageIcon className="size-5 shrink-0" aria-hidden="true" /> : null}
      <span
        className={cn(
          'max-w-full leading-tight font-bold',
          compact ? 'truncate text-xs' : 'text-lg break-words'
        )}
      >
        {label}
      </span>
      {description && !compact ? <span className="line-clamp-2 text-xs">{description}</span> : null}
    </div>
  )
}
