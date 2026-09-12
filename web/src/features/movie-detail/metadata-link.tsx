import { Link } from '@tanstack/react-router'

import { Badge } from '@/components/ui/badge'
import type { MetadataTarget } from '@/features/discover/metadata-search'
import { useDiscoverStore } from '@/stores/discover'

export function MetadataLink({
  kind,
  id,
  name,
  zone,
  badge = false
}: MetadataTarget & { badge?: boolean }) {
  const main = useDiscoverStore(state => state.category.main)
  const link = (
    <Link
      to="/discover/search"
      search={
        kind === 'tag' ? { kind, id, name, zone, page: 1, main } : { kind, id, name, page: 1, main }
      }
      className={
        badge
          ? undefined
          : 'max-w-full break-words outline-ring transition-colors hover:text-muted-foreground'
      }
    >
      {name}
    </Link>
  )
  return badge ? (
    <Badge
      variant="outline"
      className="h-auto max-w-full min-w-0 whitespace-normal break-words"
      asChild
    >
      {link}
    </Badge>
  ) : (
    link
  )
}
