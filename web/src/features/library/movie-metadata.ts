import type { LibraryMovie } from '@/api/library'
import type { MovieMetadataValues } from '@/features/movie-detail/metadata'

export function libraryMovieMetadata(movie: LibraryMovie): MovieMetadataValues {
  return {
    maker: movie.maker,
    series: movie.series,
    director: movie.director,
    actors: movie.actors ?? [],
    // Database tag IDs identify local rows, not JavDB search targets.
    tags: (movie.tags ?? []).map(tag => ({ id: tag.javdb_id, name: tag.name }))
  }
}
