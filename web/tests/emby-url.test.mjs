import assert from 'node:assert/strict'
import { test } from 'node:test'

import { combineServerUrl, splitServerUrl } from '../src/features/settings/emby-url.ts'

test('splitServerUrl extracts host and port correctly', () => {
  assert.deepEqual(splitServerUrl('http://10.32.217.101:8096'), {
    host: 'http://10.32.217.101',
    port: '8096'
  })
  assert.deepEqual(splitServerUrl('http://10.32.217.101'), {
    host: 'http://10.32.217.101',
    port: '8096'
  })
  assert.deepEqual(splitServerUrl('10.32.217.101:8096'), {
    host: '10.32.217.101',
    port: '8096'
  })
  assert.deepEqual(splitServerUrl('10.32.217.101'), {
    host: '10.32.217.101',
    port: '8096'
  })
  assert.deepEqual(splitServerUrl('https://emby.domain.com:8920'), {
    host: 'https://emby.domain.com',
    port: '8920'
  })
  assert.deepEqual(splitServerUrl('https://emby.domain.com'), {
    host: 'https://emby.domain.com',
    port: ''
  })
  assert.deepEqual(splitServerUrl(''), {
    host: '',
    port: '8096'
  })
  assert.deepEqual(splitServerUrl('http://10.32.217.101:8096/emby'), {
    host: 'http://10.32.217.101/emby',
    port: '8096'
  })
})

test('combineServerUrl formats full server URL properly', () => {
  assert.equal(combineServerUrl('http://10.32.217.101', '8096'), 'http://10.32.217.101:8096')
  assert.equal(combineServerUrl('10.32.217.101', '8096'), 'http://10.32.217.101:8096')
  assert.equal(combineServerUrl('https://emby.domain.com', '8920'), 'https://emby.domain.com:8920')
  assert.equal(combineServerUrl('https://emby.domain.com', ''), 'https://emby.domain.com')
  assert.equal(combineServerUrl('http://10.32.217.101:8096', '8920'), 'http://10.32.217.101:8920')
  assert.equal(combineServerUrl('', '8096'), '')
})
