import assert from 'node:assert/strict'
import { renderToStaticMarkup } from 'react-dom/server'
import { test } from 'vitest'

import { OverflowTooltip } from '@/components/overflow-tooltip'

test('overflow tooltip keeps one trigger id and stays closed initially', () => {
  const html = renderToStaticMarkup(
    <OverflowTooltip content="完整标题">
      <h3>短标题</h3>
    </OverflowTooltip>
  )

  assert.match(html, /<h3[^>]*id="[^"]+"/)
  assert.doesNotMatch(html, /完整标题/)
})
