import { useEffect, useState } from 'react'
import { XIcon } from 'lucide-react'

import type { LibraryFilterOptions } from '@/api/library'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { FilterMultiSelect } from './filter-multi-select'
import type { MoviesFilter } from './types'

const SORT_OPTIONS = [
  { value: 'recent', label: '最近加入' },
  { value: 'rating', label: '评分最高' },
  { value: 'release', label: '发行日期' }
] as const

const SOURCE_OPTIONS = [
  { value: 'all', label: '全部来源' },
  { value: '115', label: '115' },
  { value: 'local', label: '本地' }
] as const

export function MovieFilterBar({
  options,
  value,
  onChange
}: {
  options?: LibraryFilterOptions
  value: MoviesFilter
  onChange: (next: MoviesFilter) => void
}) {
  const [query, setQuery] = useState(value.q ?? '')

  useEffect(() => {
    const current = value.q ?? ''
    if (query === current) return
    const timer = setTimeout(() => onChange({ ...value, q: query || undefined }), 300)
    return () => clearTimeout(timer)
  }, [query, value, onChange])

  const sortValue = value.sort ?? 'recent'
  const sourceValue = value.source ?? 'all'
  const sortLabel = SORT_OPTIONS.find(option => option.value === sortValue)?.label ?? '最近加入'
  const sourceLabel = SOURCE_OPTIONS.find(option => option.value === sourceValue)?.label ?? '全部来源'

  const hasFilters =
    Boolean(value.q) ||
    Boolean(value.source) ||
    (value.tagNames?.length ?? 0) > 0 ||
    (value.actorNames?.length ?? 0) > 0 ||
    (value.makerNames?.length ?? 0) > 0 ||
    (value.seriesNames?.length ?? 0) > 0

  const reset = () => {
    setQuery('')
    onChange({ sort: value.sort })
  }

  return (
    <div className="flex min-w-0 flex-wrap items-center gap-2">
      <Input
        className="h-8 w-full sm:w-60"
        placeholder="搜索番号或标题"
        value={query}
        onChange={event => setQuery(event.target.value)}
      />

      <Select
        value={sortValue}
        onValueChange={next =>
          onChange({ ...value, sort: (next ?? 'recent') as MoviesFilter['sort'] })
        }
      >
        <SelectTrigger className="w-32">
          <SelectValue>{sortLabel}</SelectValue>
        </SelectTrigger>
        <SelectContent align="start">
          <SelectGroup>
            {SORT_OPTIONS.map(option => (
              <SelectItem key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>

      <Select
        value={sourceValue}
        onValueChange={next =>
          onChange({
            ...value,
            source: next === 'all' ? undefined : (next as MoviesFilter['source'])
          })
        }
      >
        <SelectTrigger className="w-28">
          <SelectValue>{sourceLabel}</SelectValue>
        </SelectTrigger>
        <SelectContent align="start">
          <SelectGroup>
            {SOURCE_OPTIONS.map(option => (
              <SelectItem key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>

      <FilterMultiSelect
        label="标签"
        options={options?.tags ?? []}
        value={value.tagNames ?? []}
        onChange={next => onChange({ ...value, tagNames: next.length ? next : undefined })}
      />
      <FilterMultiSelect
        label="演员"
        options={options?.actors ?? []}
        value={value.actorNames ?? []}
        onChange={next => onChange({ ...value, actorNames: next.length ? next : undefined })}
      />
      <FilterMultiSelect
        label="制片商"
        options={options?.makers ?? []}
        value={value.makerNames ?? []}
        onChange={next => onChange({ ...value, makerNames: next.length ? next : undefined })}
      />
      <FilterMultiSelect
        label="系列"
        options={options?.series ?? []}
        value={value.seriesNames ?? []}
        onChange={next => onChange({ ...value, seriesNames: next.length ? next : undefined })}
      />

      {hasFilters ? (
        <Button type="button" variant="ghost" onClick={reset}>
          <XIcon className="size-4" />
          清除
        </Button>
      ) : null}
    </div>
  )
}
