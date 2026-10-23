import { Maximize2Icon } from 'lucide-react'
import { useState } from 'react'

import type { PreviewImage } from '@/api/discover'
import { MediaImage } from '@/components/media-image'
import { MoviePreviewViewer } from './preview-viewer'

export function MoviePreviews({ images, label }: { images: PreviewImage[]; label: string }) {
  // The viewer stays mounted so its closing animation can play; the index outlives `open`.
  const [viewerOpen, setViewerOpen] = useState(false)
  const [viewerIndex, setViewerIndex] = useState(0)

  const openViewer = (index: number) => {
    setViewerIndex(index)
    setViewerOpen(true)
  }

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
            <div
              key={`${preview.original}:${index}`}
              className="group relative aspect-video overflow-hidden rounded-xl"
            >
              <MediaImage
                source={preview.thumbnail || preview.original}
                original={preview.original}
                label={label}
                description={`预览图 ${index + 1}`}
                className="object-cover"
              />
              <button
                type="button"
                aria-label={`放大 ${label} 预览图 ${index + 1}`}
                onClick={() => openViewer(index)}
                className="absolute top-1 right-1 cursor-pointer rounded-md bg-black/60 p-1.5 text-white"
              >
                <Maximize2Icon className="size-4" />
              </button>
            </div>
          ))}
        </div>
      )}
      <MoviePreviewViewer
        images={images}
        label={label}
        index={viewerIndex}
        open={viewerOpen}
        onIndexChange={setViewerIndex}
        onClose={() => setViewerOpen(false)}
      />
    </section>
  )
}
