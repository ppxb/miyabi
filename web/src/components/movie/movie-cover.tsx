import { MediaImage } from '@/components/media-image'

export function MovieCover({
  source,
  loading = 'lazy'
}: {
  source: string
  loading?: 'eager' | 'lazy'
}) {
  return <MediaImage source={source} loading={loading} className="object-contain" />
}
