import assert from 'node:assert/strict'
import { test } from 'vitest'

import { getPageNumbers } from '@/lib/pagination'

test('small totals list every page', () => {
  assert.deepEqual(getPageNumbers(1, 1), [1])
  assert.deepEqual(getPageNumbers(2, 2), [1, 2])
  assert.deepEqual(getPageNumbers(1, 4), [1, 2, 3, 4])
  assert.deepEqual(getPageNumbers(3, 5), [1, 2, 3, 4, 5])
  assert.deepEqual(getPageNumbers(1, 7), [1, 2, 3, 4, 5, 6, 7])
  assert.deepEqual(getPageNumbers(1, 8), [1, 2, 3, 'ellipsis-right', 8])
})

test('near the start shows a 3-page window and the last page', () => {
  assert.deepEqual(getPageNumbers(1, 30), [1, 2, 3, 'ellipsis-right', 30])
  assert.deepEqual(getPageNumbers(2, 30), [1, 2, 3, 'ellipsis-right', 30])
})

test('an ellipsis never hides a single page', () => {
  assert.deepEqual(getPageNumbers(3, 30), [1, 2, 3, 4, 'ellipsis-right', 30])
  assert.deepEqual(getPageNumbers(4, 30), [1, 2, 3, 4, 5, 'ellipsis-right', 30])
  assert.deepEqual(getPageNumbers(27, 30), [1, 'ellipsis-left', 26, 27, 28, 29, 30])
})

test('in the middle shows first, window, last with both ellipses', () => {
  assert.deepEqual(getPageNumbers(5, 30), [1, 'ellipsis-left', 4, 5, 6, 'ellipsis-right', 30])
  assert.deepEqual(getPageNumbers(15, 30), [1, 'ellipsis-left', 14, 15, 16, 'ellipsis-right', 30])
})

test('near the end shows the first page and a 3-page window', () => {
  assert.deepEqual(getPageNumbers(29, 30), [1, 'ellipsis-left', 28, 29, 30])
  assert.deepEqual(getPageNumbers(30, 30), [1, 'ellipsis-left', 28, 29, 30])
})

test('out-of-range and fractional input is clamped', () => {
  assert.deepEqual(getPageNumbers(99, 30), [1, 'ellipsis-left', 28, 29, 30])
  assert.deepEqual(getPageNumbers(0, 30), [1, 2, 3, 'ellipsis-right', 30])
  assert.deepEqual(getPageNumbers(2, 4.9), [1, 2, 3, 4])
})
