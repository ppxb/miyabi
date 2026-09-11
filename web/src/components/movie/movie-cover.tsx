import { MediaImage } from '@/components/media-image'

export function MovieCover({
  source,
  loading = 'lazy',
  onReady
}: {
  source: string
  loading?: 'eager' | 'lazy'
  onReady?: () => void
}) {
  return (
    <MediaImage
      source={source}
      loading={loading}
      onReady={onReady}
      blurredBackground
      className="object-contain"
    />
  )
}
