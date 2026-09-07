import type { PreviewImage } from '@/api/discover'
import { MediaImage } from '@/components/media-image'

export function MoviePreviews({ code, images }: { code: string; images: PreviewImage[] }) {
  return (
    <section className="space-y-4">
      <h2 id="movie-previews-title" className="text-xl font-semibold tracking-normal">
        预览图
      </h2>
      {images.length === 0 ? (
        <p className="text-sm text-muted-foreground">暂无预览图</p>
      ) : (
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-4 lg:grid-cols-8">
          {images.map((preview, index) => (
            <div key={preview.original} className="aspect-video overflow-hidden rounded-lg">
              <MediaImage
                source={preview.thumbnail}
                original={preview.original}
                alt={`${code} 预览图 ${index + 1}`}
                className="object-cover"
              />
            </div>
          ))}
        </div>
      )}
    </section>
  )
}
