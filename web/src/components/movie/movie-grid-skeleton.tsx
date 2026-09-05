import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { MovieGridLayout } from './movie-grid'

export function MovieGridSkeleton({ count = 8 }: { count?: number }) {
  return (
    <MovieGridLayout>
      {Array.from({ length: count }, (_, index) => (
        <MovieCardSkeleton key={index} />
      ))}
    </MovieGridLayout>
  )
}

export function MovieCardSkeleton() {
  return (
    <Card size="sm" className="h-full gap-0 overflow-hidden py-0">
      <Skeleton className="aspect-3/2 w-full rounded-none" />
      <CardContent className="space-y-2 p-3">
        <Skeleton className="h-5 w-full" />
        <Skeleton className="h-4 w-3/4" />
        <Skeleton className="h-5 w-1/2" />
      </CardContent>
    </Card>
  )
}
