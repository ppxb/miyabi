import { createFileRoute } from '@tanstack/react-router'

import { ActorDetailPage } from '@/features/actors/detail-page'

export const Route = createFileRoute('/actors/$key')({
  component: ActorDetailRoute
})

function ActorDetailRoute() {
  const { key } = Route.useParams()
  return <ActorDetailPage actorKey={key} />
}
