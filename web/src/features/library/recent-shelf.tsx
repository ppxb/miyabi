import { useRecentMovies } from '@/api/library'
import { MovieShelfSection } from '@/features/library/movie-shelf-section'

export function RecentlyAddedShelf({ onPlay }: { onPlay: (movieID: number) => void }) {
  const items = useRecentMovies().data?.movies ?? []
  return <MovieShelfSection title="最近加入" movies={items} onPlay={onPlay} />
}
