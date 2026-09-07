import { Link } from '@tanstack/react-router'

import { Badge } from '@/components/ui/badge'
import type { MetadataTarget } from '@/features/discover/metadata-search'

export function MetadataLink({
  kind,
  id,
  name,
  zone,
  badge = false
}: MetadataTarget & { badge?: boolean }) {
  const link = (
    <Link
      to="/discover/search"
      search={kind === 'tag' ? { kind, id, name, zone, page: 1 } : { kind, id, name, page: 1 }}
      className={badge ? undefined : 'outline-ring transition-colors hover:text-muted-foreground'}
    >
      {name}
    </Link>
  )
  return badge ? (
    <Badge variant="outline" asChild>
      {link}
    </Badge>
  ) : (
    link
  )
}
