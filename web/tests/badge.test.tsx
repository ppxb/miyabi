import assert from 'node:assert/strict'
import { renderToStaticMarkup } from 'react-dom/server'
import { test } from 'vitest'

import { Badge } from '@/components/ui/badge'

test('badge renders its children', () => {
  const html = renderToStaticMarkup(<Badge variant="frosted">ABC-123</Badge>)
  assert.match(html, /ABC-123/)
})
