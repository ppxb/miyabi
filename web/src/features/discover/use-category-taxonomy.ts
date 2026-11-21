import type { Zone } from '@/api/discover'
import { useDiscoverTags } from '@/api/discover'
import { MAIN_CATEGORY, UNSUPPORTED_CATEGORIES } from './constants'

/** Tags taxonomy for a discovery zone, with the main category split out. */
export function useCategoryTaxonomy(zone: Zone, categoryID?: string) {
  const taxonomy = useDiscoverTags(zone)
  const mainOptions = taxonomy.data?.find(item => item.id === MAIN_CATEGORY)?.tags ?? []
  const categories = (taxonomy.data ?? []).filter(
    item => item.id !== MAIN_CATEGORY && !UNSUPPORTED_CATEGORIES.has(item.id)
  )
  const selectedCategory = categories.find(item => item.id === categoryID) ?? categories[0]
  return { taxonomy, mainOptions, categories, selectedCategory }
}
