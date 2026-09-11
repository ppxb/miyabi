import type { BrowseMoviesParams } from '@/api/discover'
import type { CategoryFilters } from '@/stores/discover'

export function categoryBrowseParams({
  zone,
  categoryID,
  tagID,
  main
}: CategoryFilters): BrowseMoviesParams {
  return {
    zone,
    ...(main ? { main: [main] } : {}),
    ...(tagID ? (categoryID === 'year' ? { year: tagID } : { tagIds: [tagID] }) : {})
  }
}
