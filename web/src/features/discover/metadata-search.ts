import type { JavDBEntityType, JavDBZone } from '@/api/discover'
import { DISCOVER_ZONES } from './constants'

export type MetadataSearchKind = JavDBEntityType | 'tag'

export type MetadataTarget = {
  id: string
  name: string
} & ({ kind: JavDBEntityType; zone?: never } | { kind: 'tag'; zone?: JavDBZone })

export type MetadataSearch = MetadataTarget & { page: number; main: string }

export const METADATA_LABELS: Record<MetadataSearchKind, string> = {
  maker: '厂牌',
  series: '系列',
  actor: '演员',
  director: '导演',
  tag: '标签'
}

export function validateMetadataSearch(search: Record<string, unknown>): MetadataSearch {
  const { kind, id, name } = search
  const page = Number(search.page ?? 1)
  const main = search.main ?? ''
  if (
    typeof kind !== 'string' ||
    !Object.hasOwn(METADATA_LABELS, kind) ||
    typeof id !== 'string' ||
    id.length === 0 ||
    typeof name !== 'string' ||
    name.length === 0 ||
    typeof main !== 'string' ||
    !['', 'p', 'm', 'c', 's', 'i', 'v'].includes(main) ||
    !Number.isInteger(page) ||
    page < 1
  ) {
    throw new Error('搜索条件无效')
  }
  if (kind === 'tag') {
    const { zone } = search
    if (zone !== undefined && !DISCOVER_ZONES.some(item => item.value === zone)) {
      throw new Error('搜索条件无效')
    }
    return { kind, id, name, ...(zone ? { zone: zone as JavDBZone } : {}), page, main }
  }
  return { kind: kind as JavDBEntityType, id, name, page, main }
}
