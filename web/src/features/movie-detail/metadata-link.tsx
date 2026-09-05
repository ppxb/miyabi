import { Link } from '@tanstack/react-router'

import type { JavDBZone } from '@/api/discover'
import { Badge } from '@/components/ui/badge'
import type { MetadataSearchKind } from '@/features/discover/metadata-search'

export function MetadataLink({
  kind,
  id,
  name,
  zone,
  badge = false
}: {
  kind: MetadataSearchKind
  id: string
  name: string
  zone: JavDBZone
  badge?: boolean
}) {
  const link = (
    <Link
      to="/discover/search"
      search={{ kind, id, name, zone, page: 1 }}
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
