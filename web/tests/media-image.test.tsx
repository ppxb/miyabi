import {
  Children,
  isValidElement,
  useEffect,
  useState,
  type ComponentProps,
  type ReactElement,
  type ReactNode
} from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { useRouter } from '@tanstack/react-router'
import { beforeEach, expect, onTestFinished, test, vi } from 'vitest'

import { imageURL } from '@/api/client'
import { MediaImage, type MediaImageProps } from '@/components/media-image'
import { useSettingsStore, type ImageSafety } from '@/stores/settings'

vi.mock('react', async importOriginal => ({
  ...(await importOriginal<typeof import('react')>()),
  useState: vi.fn(),
  useEffect: vi.fn()
}))

vi.mock('@tanstack/react-router', () => ({ useRouter: vi.fn() }))
vi.mock('@/stores/settings', () => ({ useSettingsStore: vi.fn() }))

const source = 'https://example.com/cover.jpg'
const original = 'https://example.com/original.jpg'
const label = 'TEST-001'
const description = '封面图片'

beforeEach(() => {
  vi.clearAllMocks()
})

function paths(node: ReactNode, type: string): ReactElement<{ children?: ReactNode }>[][] {
  const result: ReactElement<{ children?: ReactNode }>[][] = []
  for (const child of Children.toArray(node)) {
    if (!isValidElement<{ children?: ReactNode }>(child)) continue
    if (child.type === type) result.push([child])
    for (const path of paths(child.props.children, type)) result.push([child, ...path])
  }
  return result
}

function elements<T extends 'img' | 'button'>(node: ReactNode, type: T) {
  return paths(node, type).map(path => path.at(-1) as ReactElement<ComponentProps<T>>)
}

function fixture(imageSafety: ImageSafety = 'blur', props: Partial<MediaImageProps> = {}) {
  let status = 'loading'
  let revealed = false
  const unsubscribe = vi.fn()
  const router = { subscribe: vi.fn(() => unsubscribe) }
  const windowTarget = new EventTarget()
  const documentTarget = new EventTarget()
  vi.stubGlobal('window', windowTarget)
  vi.stubGlobal('document', documentTarget)
  vi.mocked(useRouter).mockReturnValue(router as unknown as ReturnType<typeof useRouter>)
  vi.mocked(useSettingsStore).mockReturnValue(imageSafety)

  // This fixture exercises production callbacks, not React's scheduler. Identify state by
  // its initial value's meaning, so adding/reordering unrelated hooks does not retarget it.
  vi.mocked(useState).mockImplementation(((initial: unknown) => {
    if (initial === 'loading') {
      return [
        status,
        (value: string | ((current: string) => string)) => {
          status = typeof value === 'function' ? value(status) : value
        }
      ]
    }
    if (initial === false) {
      return [
        revealed,
        (value: boolean | ((current: boolean) => boolean)) => {
          revealed = typeof value === 'function' ? value(revealed) : value
        }
      ]
    }
    throw new Error(`Unexpected state initializer: ${String(initial)}`)
  }) as typeof useState)

  const onReady = vi.fn()
  const input = { source, original, label, description, onReady, ...props }
  function render() {
    vi.mocked(useEffect).mockClear()
    const content = MediaImage(input)
    const Content = content.type as (
      props: MediaImageProps & { imageSafety: ImageSafety }
    ) => ReactElement<ComponentProps<'div'>>
    return Content(content.props)
  }
  let tree = render()
  const f = {
    router,
    unsubscribe,
    windowTarget,
    documentTarget,
    onReady,
    get tree() {
      return tree
    },
    get images() {
      return elements(tree, 'img')
    },
    get button() {
      return elements(tree, 'button')[0]!
    },
    get markup() {
      return renderToStaticMarkup(tree)
    },
    render() {
      tree = render()
    },
    click() {
      const event = { preventDefault: vi.fn(), stopPropagation: vi.fn() }
      f.button.props.onClick!(
        event as unknown as Parameters<NonNullable<ComponentProps<'button'>['onClick']>>[0]
      )
      f.render()
      return event
    },
    readyEffect() {
      // Identify effects by their dependencies, never by their position in the hook list.
      const matches = vi
        .mocked(useEffect)
        .mock.calls.filter(([, deps]) => deps?.includes(input.onReady))
      expect(matches).toHaveLength(1)
      return matches[0]![0]()
    },
    resetEffect() {
      const matches = vi.mocked(useEffect).mock.calls.filter(([, deps]) => deps?.includes(router))
      expect(matches).toHaveLength(1)
      const cleanup = matches[0]![0]()
      let disposed = false
      const dispose = () => {
        if (disposed) return
        disposed = true
        cleanup?.()
      }
      onTestFinished(dispose)
      return dispose
    }
  }
  return f
}

function expectBlurred(tree: ReactNode) {
  const imagePaths = paths(tree, 'img')
  expect(imagePaths.length).toBeGreaterThan(0)
  for (const path of imagePaths) {
    // Both the foreground and decorative image must inherit the safety filter/scale.
    const classes = path
      .slice(0, -1)
      .map(node => (node.props as { className?: string }).className ?? '')
      .join(' ')
    const blur = /(?:^|\s)blur-\[(\d+)px\](?:\s|$)/.exec(classes)
    expect(blur).not.toBeNull()
    expect(Number(blur![1])).toBeGreaterThanOrEqual(24)
    expect(classes).toMatch(/(?:^|\s)scale-(?:1[1-9]\d|[2-9]\d\d)(?:\s|$)/)
    expect(classes).not.toMatch(
      /(?:hover:|group-hover:|peer-hover:)(?:blur-none|blur-0|filter-none)/
    )
  }
}

function expectConcealed(f: ReturnType<typeof fixture>) {
  expect(f.button.props['aria-pressed']).toBe(false)
  expect(f.button.props['aria-label']).toBe(`临时显示 ${label}`)
  expectBlurred(f.tree)
}

test('show renders images and srcSet without a reveal control or safety blur', async () => {
  const onImageLoad = vi.fn()
  const f = fixture('show', {
    blurredBackground: true,
    loading: 'eager',
    className: 'object-contain',
    onImageLoad
  })
  expect(f.images).toHaveLength(2)
  expect(elements(f.tree, 'button')).toHaveLength(0)
  for (const image of f.images) {
    expect(image.props.src).toBe(imageURL(source))
    expect(image.props.srcSet).toBe(`${imageURL(source)} 1x, ${imageURL(original)} 2x`)
    expect(image.props.loading).toBe('eager')
    expect(image.props.referrerPolicy).toBe('no-referrer')
  }
  expect(f.markup).not.toContain('blur-[32px]')
  const image = { decode: vi.fn().mockResolvedValue(undefined) } as unknown as HTMLImageElement
  await f.images.find(node => node.props.onLoad)!.props.onLoad!({ currentTarget: image } as never)
  expect(onImageLoad).toHaveBeenCalledWith(image)
  expect(image.decode).toHaveBeenCalledOnce()
  f.render()
  expect(f.markup).not.toContain('opacity-0')
  f.readyEffect()
  expect(f.onReady).toHaveBeenCalledOnce()
})

test('hide renders only the text placeholder, exposes no image sources, and signals readiness', () => {
  const f = fixture('hide', { blurredBackground: true })
  expect(f.images).toHaveLength(0)
  expect(elements(f.tree, 'button')).toHaveLength(0)
  expect(f.markup).toContain(label)
  expect(f.markup).toContain(description)
  expect(f.markup).not.toMatch(/<img\b|\bsrc(?:Set|set)?=|rel="preload"/)
  expect(f.markup).not.toContain(imageURL(source))
  expect(f.markup).not.toContain(imageURL(original))
  f.readyEffect()
  expect(f.onReady).toHaveBeenCalledOnce()
})

test('blur protects the foreground and decorative background by at least 24px plus scale', () => {
  const f = fixture('blur', { blurredBackground: true })
  expect(f.images).toHaveLength(2)
  expectConcealed(f)
  expect(f.button.props.type).toBe('button')
  expect(f.markup).toContain('点击或点按临时显示')
  expect(f.markup).not.toMatch(
    /(?:hover:|group-hover:|peer-hover:)(?:blur-none|blur-0|filter-none)/
  )
  // Hover is intentionally not an activation path; keyboard activation uses the native button.
  expect(f.tree.props.onMouseEnter).toBeUndefined()
  expect(f.tree.props.onMouseOver).toBeUndefined()
  expect(f.button.props.onMouseEnter).toBeUndefined()
  expect(f.button.props.onMouseOver).toBeUndefined()
  f.readyEffect()
  expect(f.onReady).not.toHaveBeenCalled()
})

test('click toggles reveal and its accessible state while consuming parent navigation', () => {
  const f = fixture()
  for (const pressed of [true, false]) {
    const event = f.click()
    expect(event.preventDefault).toHaveBeenCalledOnce()
    expect(event.stopPropagation).toHaveBeenCalledOnce()
    expect(f.button.props['aria-pressed']).toBe(pressed)
    expect(f.button.props['aria-label']).toBe(`${pressed ? '重新模糊' : '临时显示'} ${label}`)
    if (pressed) {
      expect(f.markup).not.toContain('blur-[32px]')
      expect(f.markup).toContain('离开或失焦后恢复模糊')
    } else expectConcealed(f)
  }
})

for (const key of ['Enter', ' ']) {
  test(`native button supports ${key === ' ' ? 'Space' : key} and prevents keyboard bubbling`, () => {
    const f = fixture()
    const event = { key, stopPropagation: vi.fn(), preventDefault: vi.fn() }
    expect(f.button.props.type).toBe('button')
    f.button.props.onKeyDown!(event as never)
    expect(event.stopPropagation).toHaveBeenCalledOnce()
    // Do not cancel the browser's native Enter/Space click generation.
    expect(event.preventDefault).not.toHaveBeenCalled()
    expectConcealed(f)
  })
}

test('leaving the image restores blur', () => {
  const f = fixture()
  f.click()
  f.tree.props.onMouseLeave!({} as never)
  f.render()
  expectConcealed(f)
})

test('focus moving within the image preserves reveal, but focus leaving resets it', () => {
  const f = fixture()
  f.click()
  const relatedTarget = {}
  const contains = vi.fn().mockReturnValue(true)
  f.tree.props.onBlur!({ currentTarget: { contains }, relatedTarget } as never)
  f.render()
  expect(contains).toHaveBeenCalledWith(relatedTarget)
  expect(f.button.props['aria-pressed']).toBe(true)
  contains.mockReturnValue(false)
  f.tree.props.onBlur!({ currentTarget: { contains }, relatedTarget: null } as never)
  f.render()
  expectConcealed(f)
})

test('concealed images do not install reset listeners', () => {
  const f = fixture()
  const windowAdd = vi.spyOn(f.windowTarget, 'addEventListener')
  const documentAdd = vi.spyOn(f.documentTarget, 'addEventListener')
  f.resetEffect()
  expect(f.router.subscribe).not.toHaveBeenCalled()
  expect(windowAdd).not.toHaveBeenCalled()
  expect(documentAdd).not.toHaveBeenCalled()
})

for (const signal of ['window blur', 'visibilitychange', 'onBeforeNavigate'] as const) {
  test(`${signal} restores blur after reveal and all reset subscriptions are cleaned up`, () => {
    const f = fixture()
    const windowAdd = vi.spyOn(f.windowTarget, 'addEventListener')
    const windowRemove = vi.spyOn(f.windowTarget, 'removeEventListener')
    const documentAdd = vi.spyOn(f.documentTarget, 'addEventListener')
    const documentRemove = vi.spyOn(f.documentTarget, 'removeEventListener')
    f.click()
    const dispose = f.resetEffect()
    expect(windowAdd).toHaveBeenCalledWith('blur', expect.any(Function))
    expect(documentAdd).toHaveBeenCalledWith('visibilitychange', expect.any(Function))
    expect(f.router.subscribe).toHaveBeenCalledWith('onBeforeNavigate', expect.any(Function))
    const navigate = (f.router.subscribe.mock.calls as unknown as [string, () => void][])[0]![1]
    const send = () => {
      if (signal === 'window blur') f.windowTarget.dispatchEvent(new Event('blur'))
      else if (signal === 'visibilitychange')
        f.documentTarget.dispatchEvent(new Event('visibilitychange'))
      else navigate()
    }
    send()
    f.render()
    expectConcealed(f)
    dispose()
    expect(f.unsubscribe).toHaveBeenCalledOnce()
    expect(windowRemove).toHaveBeenCalledWith('blur', windowAdd.mock.calls[0]![1])
    expect(documentRemove).toHaveBeenCalledWith('visibilitychange', documentAdd.mock.calls[0]![1])
    f.click()
    // After disposal, real EventTarget dispatch must no longer change component state.
    f.windowTarget.dispatchEvent(new Event('blur'))
    f.documentTarget.dispatchEvent(new Event('visibilitychange'))
    f.render()
    expect(f.button.props['aria-pressed']).toBe(true)
  })
}

test('source, original, and safety changes remount the content while cosmetic changes preserve its key', () => {
  vi.mocked(useSettingsStore).mockReturnValue('blur')
  const input = { source, original, label }
  const key = MediaImage(input).key
  expect(key).not.toBeNull()
  expect(MediaImage({ ...input, source: 'https://example.com/next.jpg' }).key).not.toBe(key)
  expect(MediaImage({ ...input, original: 'https://example.com/next-original.jpg' }).key).not.toBe(
    key
  )
  expect(MediaImage({ ...input, label: 'New label', className: 'object-cover' }).key).toBe(key)
  for (const safety of ['show', 'hide'] as const) {
    vi.mocked(useSettingsStore).mockReturnValue(safety)
    expect(MediaImage(input).key).not.toBe(key)
  }
})

for (const failure of ['network', 'decode'] as const) {
  test(`${failure} failure removes images and reveal controls, leaving a ready text placeholder`, async () => {
    const f = fixture('blur', { blurredBackground: true })
    const foreground = f.images.find(image => image.props.onLoad)!
    if (failure === 'network') foreground.props.onError!({} as never)
    else {
      const image = { decode: vi.fn().mockRejectedValue(new Error('Invalid image')) }
      await foreground.props.onLoad!({ currentTarget: image } as never)
    }
    f.render()
    expect(f.images).toHaveLength(0)
    expect(elements(f.tree, 'button')).toHaveLength(0)
    expect(f.markup).toContain(label)
    expect(f.markup).toContain(description)
    expect(f.markup).not.toMatch(/<img\b|\bsrc(?:Set|set)?=/)
    f.readyEffect()
    expect(f.onReady).toHaveBeenCalledOnce()
  })
}

test('an empty source uses the ready placeholder without requesting an image', () => {
  const f = fixture('show', { source: '', original: undefined })
  expect(f.images).toHaveLength(0)
  expect(f.markup).toContain(label)
  f.readyEffect()
  expect(f.onReady).toHaveBeenCalledOnce()
})
