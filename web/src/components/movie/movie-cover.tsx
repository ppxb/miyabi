import { MediaImage } from '@/components/media-image'

export function MovieCover({
  source,
  movie,
  loading = 'lazy',
  onReady
}: {
  source: string
  movie?: { code: string; title: string; actors?: { name: string }[] }
  loading?: 'eager' | 'lazy'
  onReady?: () => void
}) {
  return (
    <MediaImage
      source={source}
      label={movie?.code}
      description={movie?.actors?.map(actor => actor.name).join('、') || movie?.title}
      loading={loading}
      onReady={onReady}
      blurredBackground
      className="object-contain"
    />
  )
}
