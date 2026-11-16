import assert from 'node:assert/strict'
import { renderToStaticMarkup } from 'react-dom/server'
import { test } from 'vitest'

import { OnboardingStepper } from '@/features/onboarding/onboarding-stepper'

test('onboarding stepper renders the configured steps', () => {
  const html = renderToStaticMarkup(
    <OnboardingStepper
      steps={[
        { id: 'account', title: '管理员账号' },
        { id: 'source', title: '影片来源' }
      ]}
      current="source"
      onChange={() => {}}
    />
  )

  assert.match(html, /管理员账号/)
  assert.match(html, /影片来源/)
})
