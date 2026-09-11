import assert from 'node:assert/strict'
import { test } from 'node:test'

import { bindHoldSpeed } from '../src/features/player/use-hold-speed.ts'

function fixture(t, { rate = 1, delayedRate = false } = {}) {
  const window = Object.assign(new EventTarget(), {
    KeyboardEvent: class extends Event {
      constructor(type, options) {
        super(type, options)
        this.key = options.key
        this.code = options.code
        this.repeat = false
      }
    }
  })
  const document = Object.assign(new EventTarget(), { defaultView: window, hidden: false })
  const element = Object.assign(new EventTarget(), {
    ownerDocument: document,
    closest: () => null
  })
  const rates = []
  const seeks = []
  const active = []
  const forwarded = []
  let currentRate = rate
  const player = {
    el: element,
    state: { canPlay: true, canSeek: true, canSetPlaybackRate: true },
    currentTime: 60,
    duration: 600,
    get playbackRate() {
      return currentRate
    },
    set playbackRate(value) {
      rates.push(value)
      if (!delayedRate) currentRate = value
    },
    remoteControl: { seek: time => seeks.push(time) }
  }
  const dispose = bindHoldSpeed(player, value => active.push(value))
  t.after(dispose)
  for (const type of ['keydown', 'keyup']) {
    element.addEventListener(type, event => {
      if (event instanceof window.KeyboardEvent) forwarded.push(event)
    })
  }

  function key(type, { target, ...properties } = {}) {
    const event = new Event(type, { cancelable: true })
    Object.assign(event, { key: 'ArrowRight', repeat: false, ...properties })
    if (target) Object.defineProperty(event, 'target', { value: target })
    ;(type === 'keydown' ? element : window).dispatchEvent(event)
    return event
  }

  return {
    window,
    document,
    element,
    player,
    rates,
    seeks,
    active,
    forwarded,
    dispose,
    down: properties => key('keydown', properties),
    up: properties => key('keyup', properties)
  }
}

test('a short right-arrow press delegates one key pair to native seeking and feedback on release', t => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const f = fixture(t)
  assert.equal(f.down().defaultPrevented, true)
  t.mock.timers.tick(499)
  assert.deepEqual(f.seeks, [])
  assert.deepEqual(f.rates, [])
  assert.deepEqual(f.forwarded, [])
  assert.equal(f.up().defaultPrevented, true)
  assert.deepEqual(
    f.forwarded.map(event => [event.type, event.key, event.code, event.repeat]),
    [
      ['keydown', 'ArrowRight', 'ArrowRight', false],
      ['keyup', 'ArrowRight', 'ArrowRight', false]
    ]
  )
  t.mock.timers.tick(1000)
  assert.deepEqual(f.rates, [])
  assert.deepEqual(f.active, [])
  assert.deepEqual(f.seeks, [], 'the custom handler must not also seek after forwarding the key')
})

test('consecutive taps each trigger fresh native feedback without entering a held-key loop', t => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const f = fixture(t)
  f.down()
  f.up()
  f.down()
  t.mock.timers.tick(100)
  f.up()
  assert.deepEqual(
    f.forwarded.map(event => event.type),
    ['keydown', 'keyup', 'keydown', 'keyup']
  )
  assert.notEqual(f.forwarded[0], f.forwarded[2])
  assert.deepEqual(f.rates, [])
  t.mock.timers.tick(1000)
  assert.deepEqual(f.rates, [])
  assert.deepEqual(f.seeks, [])
})

test('holding right for half a second enables 3x once and suppresses repeated seeking', t => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const f = fixture(t)
  let forwarded = 0
  f.element.addEventListener('keydown', () => forwarded++)
  f.window.addEventListener('keyup', () => forwarded++)
  f.down()
  t.mock.timers.tick(250)
  f.down({ repeat: true })
  t.mock.timers.tick(250)
  assert.deepEqual(f.rates, [3])
  assert.deepEqual(f.active, [true])
  for (let i = 0; i < 10; i++) {
    assert.equal(f.down({ repeat: true }).defaultPrevented, true)
    t.mock.timers.tick(100)
  }
  f.up()
  assert.deepEqual(f.rates, [3, 1])
  assert.deepEqual(f.active, [true, false])
  assert.deepEqual(f.seeks, [])
  assert.deepEqual(f.forwarded, [])
  assert.equal(forwarded, 0)
})

test('release restores the selected speed even before the provider reports the boosted rate', t => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  for (const delayedRate of [false, true]) {
    const f = fixture(t, { rate: 1.5, delayedRate })
    f.down()
    t.mock.timers.tick(500)
    f.up()
    assert.deepEqual(f.rates, [3, 1.5])
    assert.deepEqual(f.active, [true, false])
    assert.deepEqual(f.seeks, [])
    f.dispose()
  }
})

test('focus, page and playback interruptions cancel both pending and active holds', t => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const interruptions = [
    f => f.window.dispatchEvent(new Event('blur')),
    f => f.window.dispatchEvent(new Event('pagehide')),
    f => {
      f.document.hidden = true
      f.document.dispatchEvent(new Event('visibilitychange'))
    },
    ...['focusout', 'pause', 'ended', 'error', 'source-change', 'provider-change', 'emptied'].map(
      type => f => f.element.dispatchEvent(new Event(type))
    )
  ]
  for (const interrupt of interruptions) {
    for (const elapsed of [250, 500]) {
      const f = fixture(t, { rate: 1.5 })
      f.down()
      t.mock.timers.tick(elapsed)
      interrupt(f)
      assert.equal(f.down({ repeat: true }).defaultPrevented, true)
      t.mock.timers.tick(1000)
      assert.equal(f.up().defaultPrevented, true)
      assert.deepEqual(f.rates, elapsed === 500 ? [3, 1.5] : [])
      assert.deepEqual(f.active, elapsed === 500 ? [true, false] : [])
      assert.deepEqual(f.seeks, [])
      assert.deepEqual(f.forwarded, [])
      f.dispose()
    }
  }
})

test('a new press works after focus loss swallows the old key release', t => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const f = fixture(t)
  f.down()
  t.mock.timers.tick(500)
  f.window.dispatchEvent(new Event('blur'))
  f.down({ repeat: true })
  t.mock.timers.tick(1000)
  assert.deepEqual(f.rates, [3, 1])
  f.down()
  t.mock.timers.tick(500)
  f.up()
  assert.deepEqual(f.rates, [3, 1, 3, 1])
  assert.deepEqual(f.seeks, [])
})

test('other keys, modifiers, editing fields and player menus keep their own keyboard behavior', t => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const f = fixture(t)
  const keys = [
    { key: 'ArrowLeft' },
    { key: 'ArrowUp' },
    { key: ' ' },
    { altKey: true },
    { ctrlKey: true },
    { metaKey: true },
    { shiftKey: true },
    { isComposing: true },
    ...[
      'input',
      'textarea',
      'select',
      'contenteditable',
      'select-trigger',
      'select-content',
      '.vds-menu',
      '.vds-slider'
    ].map(selector => ({
      target: { closest: selectors => (selectors.includes(selector) ? {} : null) }
    }))
  ]
  for (const key of keys) {
    assert.equal(f.down(key).defaultPrevented, false)
    t.mock.timers.tick(1000)
    assert.equal(f.up(key).defaultPrevented, false)
  }
  assert.deepEqual(f.seeks, [])
  assert.deepEqual(f.forwarded, [])
  assert.deepEqual(f.rates, [])
})

test('unmount clears the timer, restores speed and removes keyboard listeners', t => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  for (const elapsed of [250, 500]) {
    const f = fixture(t, { rate: 2 })
    f.down()
    t.mock.timers.tick(elapsed)
    f.dispose()
    t.mock.timers.tick(1000)
    assert.equal(f.down().defaultPrevented, false)
    assert.equal(f.up().defaultPrevented, false)
    assert.deepEqual(f.rates, elapsed === 500 ? [3, 2] : [])
    assert.deepEqual(f.seeks, [])
    assert.deepEqual(f.forwarded, [])
  }
})

test('unready or unsupported playback cannot start a speed boost', t => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const f = fixture(t)
  f.player.state.canPlay = false
  assert.equal(f.down().defaultPrevented, false)
  f.up()
  f.player.state.canPlay = true
  f.player.state.canSetPlaybackRate = false
  f.down()
  t.mock.timers.tick(500)
  f.up()
  assert.deepEqual(f.rates, [])
  assert.deepEqual(f.active, [])
  assert.deepEqual(f.seeks, [])
  assert.deepEqual(f.forwarded, [])
})
