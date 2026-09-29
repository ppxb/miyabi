import assert from 'node:assert/strict'
import { test } from 'vitest'

import { sameSource, sourceKey } from '@/lib/source'

test('source comparison supports scan and offline identities', () => {
  const scan = { account_id: 'account-1', directory: { id: 'directory-1' } }
  const offline = { account_id: 'account-1', directory_id: 'directory-1' }
  assert.equal(sameSource(scan, offline), true)
  assert.equal(sameSource(offline, scan), true)
  assert.equal(sourceKey(scan), sourceKey(offline))
  assert.equal(sameSource(scan, { ...offline, account_id: 'account-2' }), false)
  assert.equal(sameSource(scan, { ...offline, directory_id: 'directory-2' }), false)
  assert.equal(sameSource(undefined, scan), false)
  assert.equal(sameSource(scan, null), false)
  assert.equal(sameSource(undefined, null), false)
})
