import { createFileRoute } from '@tanstack/react-router'

import { LibraryPage } from '@/features/library/page'

export const Route = createFileRoute('/')({
  component: LibraryPage
})
