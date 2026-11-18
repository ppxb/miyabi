import { Link } from '@tanstack/react-router'
import { ChevronLeftIcon, ChevronRightIcon } from 'lucide-react'
import { useEffect, useRef, useState, type CSSProperties, type ReactNode } from 'react'

import { Button } from '@/components/ui/button'

export function MovieShelf({
  title,
  itemCount,
  href,
  children
}: {
  title: string
  itemCount: number
  href?: string
  children: ReactNode
}) {
  if (itemCount === 0) return null
  return (
    <MovieShelfContent title={title} itemCount={itemCount} href={href}>
      {children}
    </MovieShelfContent>
  )
}

function MovieShelfContent({
  title,
  itemCount,
  href,
  children
}: {
  title: string
  itemCount: number
  href?: string
  children: ReactNode
}) {
  const listRef = useRef<HTMLDivElement>(null)
  const [overflow, setOverflow] = useState(false)
  const [atStart, setAtStart] = useState(true)
  const [atEnd, setAtEnd] = useState(true)

  useEffect(() => {
    const list = listRef.current
    if (!list) return
    const update = () => {
      const maxScroll = list.scrollWidth - list.clientWidth
      const edge = 16
      setOverflow(maxScroll > 1)
      setAtStart(list.scrollLeft <= edge)
      setAtEnd(list.scrollLeft >= maxScroll - edge)
    }
    update()
    const observer = new ResizeObserver(update)
    observer.observe(list)
    list.addEventListener('scroll', update, { passive: true })
    return () => {
      observer.disconnect()
      list.removeEventListener('scroll', update)
    }
  }, [itemCount])

  const scroll = (direction: -1 | 1) => {
    const list = listRef.current
    if (!list) return
    list.scrollBy({ left: direction * list.clientWidth, behavior: 'smooth' })
  }
  const startFade = atStart ? '0px' : '2rem'
  const endFade = atEnd ? '0px' : '2rem'
  const mask = `linear-gradient(to right, transparent 0, #000 ${startFade}, #000 calc(100% - ${endFade}), transparent 100%)`

  return (
    <section className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-lg font-semibold tracking-normal">{title}</h2>
        <div className="flex items-center gap-1">
          {overflow ? (
            <>
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                aria-label="向左滚动"
                disabled={atStart}
                onClick={() => scroll(-1)}
              >
                <ChevronLeftIcon />
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                aria-label="向右滚动"
                disabled={atEnd}
                onClick={() => scroll(1)}
              >
                <ChevronRightIcon />
              </Button>
            </>
          ) : null}
          {href ? (
            <Button variant="default" size="sm" nativeButton={false} render={<Link to={href} />}>
              查看全部
            </Button>
          ) : null}
        </div>
      </div>
      <div
        ref={listRef}
        style={
          {
            maskImage: mask,
            WebkitMaskImage: mask,
            maskRepeat: 'no-repeat',
            WebkitMaskRepeat: 'no-repeat',
            animation: 'none'
          } as CSSProperties
        }
        className="flex scroll-fade-x snap-x snap-mandatory [scrollbar-width:none] gap-3 overflow-x-auto px-1 py-1 [&::-webkit-scrollbar]:hidden"
      >
        {children}
      </div>
    </section>
  )
}
