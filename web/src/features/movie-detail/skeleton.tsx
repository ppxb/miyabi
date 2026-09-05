import { Skeleton } from '@/components/ui/skeleton'
import { MovieGridSkeleton } from '@/components/movie'

export function MovieDetailSkeleton() {
  return (
    <div role="status" aria-label="正在加载影片详情" className="space-y-10">
      <section className="grid items-start gap-6 lg:grid-cols-[minmax(0,1.1fr)_minmax(0,1fr)] lg:gap-8">
        <Skeleton className="aspect-3/2 w-full rounded-2xl" />
        <div className="space-y-5 py-1">
          <Skeleton className="h-5 w-24 rounded-full" />
          <div className="space-y-3">
            <Skeleton className="h-8 w-full" />
            <Skeleton className="h-8 w-4/5" />
          </div>
          <div className="grid grid-cols-3 gap-3">
            {Array.from({ length: 3 }, (_, index) => (
              <Skeleton key={index} className="h-20 rounded-2xl" />
            ))}
          </div>
          <Skeleton className="h-5 w-2/3" />
          <Skeleton className="h-5 w-1/2" />
          <Skeleton className="h-5 w-3/4" />
          <Skeleton className="h-6 w-4/5 rounded-full" />
        </div>
      </section>
      <div className="space-y-4">
        <Skeleton className="h-7 w-20" />
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-4 lg:grid-cols-8">
          {Array.from({ length: 8 }, (_, index) => (
            <Skeleton key={index} className="aspect-video rounded-lg" />
          ))}
        </div>
      </div>
      <div className="space-y-4">
        <Skeleton className="h-7 w-20" />
        <div className="space-y-2">
          {Array.from({ length: 3 }, (_, index) => (
            <Skeleton key={index} className="h-30 rounded-2xl" />
          ))}
        </div>
      </div>
      {Array.from({ length: 2 }, (_, section) => (
        <div key={section} className="space-y-4">
          <Skeleton className="h-7 w-40" />
          <MovieGridSkeleton count={8} />
        </div>
      ))}
    </div>
  )
}
