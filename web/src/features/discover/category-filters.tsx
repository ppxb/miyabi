import type { Zone } from '@/api/discover'
import { InlineError } from '@/components/error-state'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import type { CategoryFilters as CategoryValues } from './search'
import { CommonFilterSelect } from './common-filter-select'
import { DISCOVER_ZONES as zones } from './constants'
import { useCategoryTaxonomy } from './use-category-taxonomy'

export function CategoryFilters({
  category,
  updateCategory
}: {
  category: CategoryValues
  updateCategory: (filters: Partial<CategoryValues>) => void
}) {
  const { zone, categoryID, tagID, main } = category
  const { taxonomy, mainOptions, categories, selectedCategory } = useCategoryTaxonomy(
    zone,
    categoryID
  )
  const effectiveCategoryID = selectedCategory?.id ?? ''
  const categoryItems = categories.map(item => ({ value: item.id, label: item.name }))
  const tagItems = [
    { value: 'all', label: '全部标签' },
    ...(selectedCategory?.tags ?? []).map(tag => ({ value: tag.id, label: tag.name }))
  ]

  return (
    <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap">
      <Select
        value={zone}
        items={zones}
        onValueChange={value =>
          updateCategory({ zone: (value ?? '') as Zone, categoryID: '', tagID: '', main: '' })
        }
      >
        <SelectTrigger className="w-full sm:w-32">
          <SelectValue />
        </SelectTrigger>
        <SelectContent align="start">
          <SelectGroup>
            {zones.map(item => (
              <SelectItem key={item.value} value={item.value}>
                {item.label}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>

      {taxonomy.isLoading ? (
        <>
          <Skeleton className="h-8 w-full rounded-lg sm:w-48" />
          <Skeleton className="h-8 w-full rounded-lg sm:w-56" />
          <Skeleton className="h-8 w-full rounded-lg sm:w-48" />
        </>
      ) : taxonomy.isError ? (
        <InlineError
          onRetry={() => taxonomy.refetch()}
          retrying={taxonomy.isFetching}
          retryLabel="重试分类"
        >
          分类加载失败
        </InlineError>
      ) : (
        <>
          <Select
            value={effectiveCategoryID}
            items={categoryItems}
            onValueChange={value => updateCategory({ categoryID: value ?? '', tagID: '' })}
          >
            <SelectTrigger className="w-full sm:w-48">
              <SelectValue placeholder="选择分类" />
            </SelectTrigger>
            <SelectContent align="start">
              <SelectGroup>
                {categories.map(category => (
                  <SelectItem key={category.id} value={category.id}>
                    {category.name}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
          <Select
            value={tagID || 'all'}
            items={tagItems}
            onValueChange={value =>
              updateCategory({
                categoryID: effectiveCategoryID,
                tagID: value === 'all' ? '' : (value ?? '')
              })
            }
          >
            <SelectTrigger className="w-full sm:w-56">
              <SelectValue />
            </SelectTrigger>
            <SelectContent align="start">
              <SelectGroup>
                <SelectItem value="all">全部标签</SelectItem>
                {selectedCategory?.tags.map(tag => (
                  <SelectItem key={tag.id} value={tag.id}>
                    {tag.name}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
          {mainOptions.length > 0 ? (
            <CommonFilterSelect
              options={mainOptions}
              value={main}
              onValueChange={mainValue => updateCategory({ main: mainValue ?? '' })}
            />
          ) : null}
        </>
      )}
    </div>
  )
}
