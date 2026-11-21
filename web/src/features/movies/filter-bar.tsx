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
  const [syncedQ, setSyncedQ] = useState(value.q ?? '')

  // Reflect external URL changes (back/forward) into the input, but never erase
  // what the user is still typing: skip the write when the URL value merely
  // echoes our own trimmed submission ("abc " -> "abc").
  const externalQ = value.q ?? ''
  if (externalQ !== syncedQ) {
    setSyncedQ(externalQ)
    if (externalQ !== query.trim()) {
      setQuery(externalQ)
    }
  }

  useEffect(() => {
    const current = value.q ?? ''
    if (query.trim() === current) return
    const timer = setTimeout(() => onChange({ ...value, q: query.trim() || undefined }), 300)
    return () => clearTimeout(timer)
  }, [query, value, onChange])

  const sortValue = value.sort ?? 'recent'
  const sourceValue = value.source ?? 'all'

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
        aria-label="搜索番号或标题"
        className="w-full sm:w-60"
        placeholder="搜索番号或标题"
        value={query}
        onChange={event => setQuery(event.target.value)}
      />

      <Select
        items={SORT_OPTIONS}
        value={sortValue}
        onValueChange={next =>
          onChange({ ...value, sort: (next ?? 'recent') as MoviesFilter['sort'] })
        }
      >
        <SelectTrigger className="w-32">
          <SelectValue />
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
        items={SOURCE_OPTIONS}
        value={sourceValue}
        onValueChange={next =>
          onChange({
            ...value,
            source: next === 'all' ? undefined : (next as MoviesFilter['source'])
          })
        }
      >
        <SelectTrigger className="w-28">
          <SelectValue />
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
