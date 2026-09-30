import assert from 'node:assert/strict'
import { renderToStaticMarkup } from 'react-dom/server'
import { test } from 'vitest'

import { ErrorState, InlineError } from '@/components/error-state'

test('errors without a retry action do not leave an empty action container', () => {
  for (const element of [
    <ErrorState key="page" message="失败" />,
    <InlineError key="inline">失败</InlineError>
  ]) {
    const html = renderToStaticMarkup(element)
    assert.equal(html.match(/<div\b/g)?.length, 1)
    assert.doesNotMatch(html, /<button\b/)
    assert.match(html, /失败/)
  }
})

test('both error variants render the retry action with its loading state and label', () => {
  const retry = { onRetry: () => {}, retrying: true, retryLabel: '重新加载' }
  for (const element of [
    <ErrorState key="page" message="失败" {...retry} />,
    <InlineError key="inline" {...retry}>
      失败
    </InlineError>
  ]) {
    const html = renderToStaticMarkup(element)
    assert.equal(html.match(/<button\b/g)?.length, 1)
    assert.match(html, /<button\b[^>]*disabled=""/)
    assert.match(html, /重新加载/)
  }
})
