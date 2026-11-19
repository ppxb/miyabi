import { createFileRoute } from '@tanstack/react-router'

import { ActorsCatalogPage } from '@/features/actors/catalog-page'

export const Route = createFileRoute('/actors/')({
  component: ActorsCatalogPage
})
