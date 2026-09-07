import { MediaImage } from '@/components/media-image'

export function MovieCover({
  source,
  alt,
  loading = 'lazy'
}: {
  source: string
  alt: string
  loading?: 'eager' | 'lazy'
}) {
  return <MediaImage source={source} alt={alt} loading={loading} className="object-contain" />
}
