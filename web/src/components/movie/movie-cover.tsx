import { MediaImage } from '@/components/media-image'

export function MovieCover({
  source,
  loading = 'lazy',
  width
}: {
  source: string
  loading?: 'eager' | 'lazy'
  width?: number
}) {
  return (
    <MediaImage
      source={source}
      loading={loading}
      width={width}
      blurredBackground
      className="object-contain"
    />
  )
}
