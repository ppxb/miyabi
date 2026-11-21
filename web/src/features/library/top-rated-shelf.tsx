import { useTopRatedMovies } from '@/api/library'
import { MovieShelfSection } from '@/features/library/movie-shelf-section'

export function TopRatedShelf({ onPlay }: { onPlay: (movieID: number) => void }) {
  const items = useTopRatedMovies(12).data ?? []
  return <MovieShelfSection title="高分影片" movies={items} onPlay={onPlay} />
}
