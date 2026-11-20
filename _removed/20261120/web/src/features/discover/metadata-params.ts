import type { BrowseMoviesParams } from '@/api/discover'
import type { MetadataSearch } from './metadata-search'

export function metadataBrowseParams(search: MetadataSearch): BrowseMoviesParams {
  return {
    ...(search.kind === 'tag'
      ? { ...(search.zone ? { zone: search.zone } : {}), tagIds: [search.id] }
      : { entityType: search.kind, entityID: search.id }),
    ...(search.main ? { main: [search.main] } : {})
  }
}
