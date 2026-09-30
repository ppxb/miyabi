import { AppPage } from '@/components/app-page'
import { PageBackButton } from '@/components/page-back-button'
import { MovieDetailContent } from './content'

export function MovieDetailPage({ movieId }: { movieId: string }) {
  return (
    <AppPage className="sm:px-6 lg:px-8" contentClassName="max-w-7xl gap-8">
      <PageBackButton />

      <MovieDetailContent movieId={movieId} />
    </AppPage>
  )
}
