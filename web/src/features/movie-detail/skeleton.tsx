import { Skeleton } from '@/components/ui/skeleton'
import { Card, CardContent } from '@/components/ui/card'
import { MovieGridSkeleton } from '@/components/movie'

export function MovieDetailSkeleton() {
  return (
    <div className="space-y-10">
      <section className="grid items-start gap-6 lg:grid-cols-[minmax(0,1.1fr)_minmax(0,1fr)] lg:gap-8">
        <Skeleton className="aspect-3/2 w-full rounded-2xl" />
        <div className="min-w-0 space-y-5 py-1">
          <div className="space-y-3">
            <div className="flex flex-wrap items-center gap-2">
              <Skeleton className="h-5 w-24 rounded-full" />
              <Skeleton className="h-5 w-16 rounded-full" />
            </div>
            <div className="space-y-1">
              <Skeleton className="h-7 w-full sm:h-8" />
              <Skeleton className="h-7 w-4/5 sm:h-8" />
            </div>
          </div>
          <div className="flex flex-wrap divide-x divide-border/60 rounded-2xl bg-muted py-1 ring-1 ring-border dark:bg-card/60 dark:ring-0">
            {Array.from({ length: 3 }, (_, index) => (
              <div
                key={index}
                className="flex min-w-0 flex-1 flex-col items-center gap-2 p-3 sm:p-4"
              >
                <Skeleton className="h-4 w-16 max-w-full bg-background/70 dark:bg-muted" />
                <Skeleton className="h-5 w-20 max-w-full bg-background/70 sm:h-6 dark:bg-muted" />
              </div>
            ))}
          </div>
          <div className="flex flex-wrap gap-2">
            <Skeleton className="h-5 w-12 rounded-full" />
            <Skeleton className="h-5 w-16 rounded-full" />
          </div>
          <div className="space-y-3">
            {['w-2/3', 'w-1/2', 'w-3/4'].map(width => (
              <div key={width} className="flex min-h-6 items-center gap-4">
                <Skeleton className="h-4 w-10 shrink-0" />
                <div className="min-w-0 flex-1">
                  <Skeleton className={`h-4 ${width}`} />
                </div>
              </div>
            ))}
            <div className="flex items-start gap-4">
              <Skeleton className="mt-0.5 h-4 w-10 shrink-0" />
              <div className="flex min-w-0 flex-wrap gap-2">
                <Skeleton className="h-5 w-16 rounded-full" />
                <Skeleton className="h-5 w-24 rounded-full" />
                <Skeleton className="h-5 w-20 rounded-full" />
              </div>
            </div>
          </div>
        </div>
      </section>
      <div className="space-y-4">
        <Skeleton className="h-7 w-20" />
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-4 lg:grid-cols-8">
          {Array.from({ length: 8 }, (_, index) => (
            <Skeleton key={index} className="aspect-video rounded-xl" />
          ))}
        </div>
      </div>
      <div className="space-y-4">
        <Skeleton className="h-7 w-20" />
        <MovieMagnetsSkeleton />
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

export function MovieMagnetsSkeleton() {
  return (
    <div className="space-y-4">
      {Array.from({ length: 3 }, (_, index) => (
        <Card key={index} size="sm">
          <CardContent className="space-y-3">
            <div className="flex flex-col gap-4 sm:flex-row sm:items-center">
              <div className="min-w-0 flex-1 space-y-2">
                <Skeleton className="h-6 w-4/5" />
                <Skeleton className="h-4 w-full" />
                <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
                  <Skeleton className="h-4 w-12" />
                  <Skeleton className="h-5 w-14 rounded-full" />
                  <Skeleton className="h-5 w-10 rounded-full" />
                </div>
              </div>
              <div className="flex shrink-0 items-center gap-2 self-end sm:self-center">
                <Skeleton className="h-8 w-16 rounded-full" />
                <Skeleton className="h-8 w-28 rounded-full" />
              </div>
            </div>
          </CardContent>
        </Card>
      ))}
    </div>
  )
}
