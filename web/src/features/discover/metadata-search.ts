import type { JavDBEntityType, JavDBZone } from '@/api/discover'
import { DISCOVER_ZONES } from './constants'

export type MetadataSearchKind = JavDBEntityType | 'tag'

export type MetadataSearch = {
  kind: MetadataSearchKind
  id: string
  name: string
  zone: JavDBZone
  page: number
}

export const METADATA_LABELS: Record<MetadataSearchKind, string> = {
  maker: '厂牌',
  series: '系列',
  actor: '演员',
  director: '导演',
  tag: '标签'
}

export function validateMetadataSearch(search: Record<string, unknown>): MetadataSearch {
  const { kind, id, name, zone } = search
  const page = Number(search.page ?? 1)
  if (
    typeof kind !== 'string' ||
    !Object.hasOwn(METADATA_LABELS, kind) ||
    typeof id !== 'string' ||
    id.length === 0 ||
    typeof name !== 'string' ||
    name.length === 0 ||
    !DISCOVER_ZONES.some(item => item.value === zone) ||
    !Number.isInteger(page) ||
    page < 1
  ) {
    throw new Error('搜索条件无效')
  }
  return { kind: kind as MetadataSearchKind, id, name, zone: zone as JavDBZone, page }
}
