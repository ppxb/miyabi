import type { CategoryFilters as CategoryValues } from './search'
import { BrowseResults } from './browse-results'
import { CategoryFilters } from './category-filters'
import { categoryBrowseParams } from './category-params'
import { DISCOVER_PAGE_SIZE as PAGE_SIZE } from './constants'
import { useCategoryTaxonomy } from './use-category-taxonomy'

export function CategoryContent({
  category,
  onCategoryChange,
  page,
  onPageChange
}: {
  category: CategoryValues
  onCategoryChange: (filters: Partial<CategoryValues>) => void
  page: number
  onPageChange: (page: number) => void
}) {
  const { zone, categoryID } = category
  const { selectedCategory } = useCategoryTaxonomy(zone, categoryID)

  return (
    <div className="space-y-6">
      <CategoryFilters category={category} updateCategory={onCategoryChange} />
      <BrowseResults
        params={{
          page,
          limit: PAGE_SIZE,
          sort: 'release',
          order: 'desc',
          ...categoryBrowseParams({
            ...category,
            // Fall back to the requested id before taxonomy loads, so a year
            // filter is not briefly sent as a tag id.
            categoryID: selectedCategory?.id ?? category.categoryID
          })
        }}
        onPageChange={onPageChange}
      />
    </div>
  )
}
