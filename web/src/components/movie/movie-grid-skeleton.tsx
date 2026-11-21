import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { MovieGridLayout } from './movie-grid-layout'

/** Grid of card placeholders. Cover-only by default, matching the browse cards
 * (`MovieCard hideContent`); pass `withContent` for cards with a text body. */
export function MovieGridSkeleton({
  count = 8,
  withContent = false
}: {
  count?: number
  withContent?: boolean
}) {
  return (
    <MovieGridLayout>
      {Array.from({ length: count }, (_, index) => (
        <MovieCardSkeleton key={index} withContent={withContent} />
      ))}
    </MovieGridLayout>
  )
}

export function MovieCardSkeleton({ withContent = false }: { withContent?: boolean }) {
  return (
    <Card size="sm" className="h-full gap-0 py-0">
      <Skeleton className="aspect-3/2 w-full rounded-none" />
      {withContent ? (
        <CardContent className="space-y-2 p-3">
          <Skeleton className="h-5 w-full" />
          <Skeleton className="h-4 w-3/4" />
        </CardContent>
      ) : null}
    </Card>
  )
}
